// Workflow written in SciPipe.
// For more information about SciPipe, see: http://scipipe.org
package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"math"
	"path/filepath"
	"runtime"
	"strconv"
	str "strings"

	sp "github.com/scipipe/scipipe"
	spc "github.com/scipipe/scipipe/components"
)

var (
	graph      = flag.Bool("graph", false, "If this flag is specified, the workflow will just print out the workflow as a graph in dot and pdf format, and nothing else")
	maxTasks   = flag.Int("maxtasks", 4, "Max number of local cores to use")
	threads    = flag.Int("threads", 1, "Number of threads that Go is allowed to start")
	geneSet    = flag.String("geneset", "smallest1", "Gene set to use (one of smallest1, smallest3, smallest4, bowes44)")
	runSlurm   = flag.Bool("slurm", false, "Start computationally heavy jobs via SLURM")
	debug      = flag.Bool("debug", false, "Increase logging level to include DEBUG messages")
	procsRegex = flag.String("procs", "plot_summary.*", "A regex specifying which processes (by name) to run up to")

	cpSignPath = "../../bin/cpsign-0.6.14.jar"
	geneSets   = map[string][]string{
		"bowes44": []string{
			// Not available in dataset: "CHRNA1".
			// Not available in dataset: "KCNE1"
			// Instead we use MinK1 as they both share the same alias
			// "MinK", and also confirmed by Wes to be the same.
			"ADORA2A", "ADRA1A", "ADRA2A", "ADRB1", "ADRB2", "CNR1", "CNR2", "CCKAR", "DRD1", "DRD2",
			"EDNRA", "HRH1", "HRH2", "OPRD1", "OPRK1", "OPRM1", "CHRM1", "CHRM2", "CHRM3", "HTR1A",
			"HTR1B", "HTR2A", "HTR2B", "AVPR1A", "CHRNA4", "CACNA1C", "GABRA1", "KCNH2", "KCNQ1", "MINK1",
			"GRIN1", "HTR3A", "SCN5A", "ACHE", "PTGS1", "PTGS2", "MAOA", "PDE3A", "PDE4D", "LCK",
			"SLC6A3", "SLC6A2", "SLC6A4", "AR", "NR3C1",
		},
		"bowes44min100percls": []string{
			"PDE3A", "SCN5A", "CCKAR", "ADRB1", "PTGS1", "CHRM3", "CHRM2", "EDNRA", "MAOA", "LCK",
			"PTGS2", "SLC6A2", "ACHE", "CNR2", "CNR1", "ADORA2A", "OPRD1", "NR3C1", "AR", "SLC6A4",
			"OPRM1", "HTR1A", "SLC6A3", "OPRK1", "AVPR1A", "ADRB2", "DRD2", "KCNH2", "DRD1", "HTR2A",
			"CHRM1",
		},
		"bowes44min100percls_small": []string{ // These are the ones for which we want to fill up with assumed negative
			"PDE3A", "SCN5A", "CCKAR", "ADRB1", "PTGS1", "CHRM3", "CHRM2", "EDNRA", "MAOA", "LCK",
			"PTGS2", "SLC6A2", "ACHE", "CNR2", "CNR1", "ADORA2A", "OPRD1", "NR3C1", "AR", "SLC6A4",
			"OPRM1",
		},
		"bowes44min100percls_large": []string{
			"HTR1A", "SLC6A3", "OPRK1", "AVPR1A", "ADRB2", "DRD2", "KCNH2", "DRD1", "HTR2A", "CHRM1",
		},
		"smallest1": []string{
			"PDE3A",
		},
		"smallest3": []string{
			"PDE3A", "SCN5A", "CCKAR",
		},
		"smallest4": []string{
			"PDE3A", "SCN5A", "CCKAR", "ADRB1",
		},
	}
	//costVals = []string{
	//	"1",
	//	"10",
	//	"100",
	//}
	costsPerTarget = map[string][]string{
		"PDE3A":   []string{"1"},
		"SCN5A":   []string{"10"},
		"PTGS1":   []string{"1"},
		"CCKAR":   []string{"1"},
		"MAOA":    []string{"1"},
		"ADRB1":   []string{"1"},
		"CHRM3":   []string{"10"},
		"CHRM2":   []string{"1"},
		"EDNRA":   []string{"1"},
		"NR3C1":   []string{"1"},
		"AR":      []string{"1"},
		"PTGS2":   []string{"1"},
		"LCK":     []string{"10"},
		"ACHE":    []string{"1"},
		"SLC6A2":  []string{"1"},
		"CNR2":    []string{"1"},
		"OPRD1":   []string{"1"},
		"ADORA2A": []string{"1"},
		"CNR1":    []string{"1"},
		"OPRM1":   []string{"1"},
		"SLC6A4":  []string{"1"},
		"HTR1A":   []string{"1"},
		"SLC6A3":  []string{"1"},
		"OPRK1":   []string{"1"},
		"AVPR1A":  []string{"100"},
		"ADRB2":   []string{"10"},
		"DRD2":    []string{"1"},
		"KCNH2":   []string{"1"},
		"DRD1":    []string{"1"},
		"HTR2A":   []string{"1"},
		"CHRM1":   []string{"1"},
	}
	replicates = []string{
		"r1",
	}
	//replicates = []string{
	//	"r1", "r2", "r3",
	//}
)

func main() {
	// --------------------------------
	// Parse flags and stuff
	// --------------------------------
	flag.Parse()
	if *debug {
		sp.InitLogDebug()
	} else {
		sp.InitLogAudit()
	}
	if len(geneSets[*geneSet]) == 0 {
		names := []string{}
		for n := range geneSets {
			names = append(names, n)
		}
		sp.Error.Fatalf("Incorrect gene set %s specified! Only allowed values are: %s\n", *geneSet, str.Join(names, ", "))
	}
	runtime.GOMAXPROCS(*threads)

	// --------------------------------
	// Show startup messages
	// --------------------------------
	sp.Audit.Printf("Using max %d OS threads to schedule max %d tasks\n", *threads, *maxTasks)
	sp.Audit.Printf("Starting workflow for %s geneset\n", *geneSet)

	// --------------------------------
	// Initialize processes and add to runner
	// --------------------------------
	wf := sp.NewWorkflow("train_models", *maxTasks)

	dbFileName := "pubchem.chembl.dataset4publication_inchi_smiles.tsv.xz"
	dlExcapeDB := wf.NewProc("dlDB", fmt.Sprintf("wget https://zenodo.org/record/173258/files/%s -O {o:excapexz}", dbFileName))
	dlExcapeDB.SetPathStatic("excapexz", "../../raw/"+dbFileName)

	unPackDB := wf.NewProc("unPackDB", "xzcat {i:xzfile} > {o:unxzed}")
	unPackDB.SetPathReplace("xzfile", "unxzed", ".xz", "")
	unPackDB.In("xzfile").Connect(dlExcapeDB.Out("excapexz"))

	dataExcapeDB := unPackDB.Out("unxzed")
	//unPackDB.Prepend = "salloc -A snic2017-7-89 -n 2 -t 8:00:00 -J unpack_excapedb"

	// Download chemical structures and "links" (references) for *approved* (small molecule) drugs
	dlApproved := wf.NewProc("dl_approv", "curl -Lfv -o {o:zip} -u $(cat drugbank_userinfo.txt) https://www.drugbank.ca/releases/5-0-11/downloads/approved-structure-links")
	dlApproved.SetPathStatic("zip", "dat/drugbank_approved_csv.zip")
	// Unzip the above file
	unzipApproved := wf.NewProc("unzip_approved", `unzip -d dat/approved/ {i:zip}; mv "dat/approved/structure links.csv" {o:csv}`)
	unzipApproved.SetPathStatic("csv", "dat/drugbank_approved.csv")
	unzipApproved.In("zip").Connect(dlApproved.Out("zip"))

	// Download chemical structures and "links" (references) for *withdrawn* (small molecule) drugs
	dlWithdrawn := wf.NewProc("dl_withdrawn", "curl -Lfv -o {o:zip} -u $(cat drugbank_userinfo.txt) https://www.drugbank.ca/releases/5-0-11/downloads/withdrawn-structure-links")
	dlWithdrawn.SetPathStatic("zip", "dat/drugbank_withdrawn_csv.zip")
	// Unzip the above file
	unzipWithdrawn := wf.NewProc("unzip_withdrawn", `unzip -d dat/withdrawn/ {i:zip}; mv "dat/withdrawn/structure links.csv" {o:csv}`)
	unzipWithdrawn.SetPathStatic("csv", "dat/drugbank_withdrawn.csv")
	unzipWithdrawn.In("zip").Connect(dlWithdrawn.Out("zip"))

	// Extract only CHEMBL and PubChem IDs from DrugBank TSV files and merge into one file
	drugBankCompIDCmd := `csvcut -K 1 -c 11,14 {i:drugbankcsv} | sed '/^,$/d' | sed '/^$/d' | sort -V > {o:compids}`
	drugBankCompIDsApprov := wf.NewProc("drugbank_compids_appr", drugBankCompIDCmd)
	drugBankCompIDsApprov.SetPathExtend("drugbankcsv", "compids", ".compids.csv")
	drugBankCompIDsApprov.In("drugbankcsv").Connect(unzipApproved.Out("csv"))
	drugBankCompIDsWithdr := wf.NewProc("drugbank_compids_withdr", drugBankCompIDCmd)
	drugBankCompIDsWithdr.SetPathExtend("drugbankcsv", "compids", ".compids.csv")
	drugBankCompIDsWithdr.In("drugbankcsv").Connect(unzipWithdrawn.Out("csv"))

	excapeDBCompIDs := wf.NewProc("ext_excape_compids", `awk '{ print $2 }' {i:excapedb} | tail -n +2 | sort -V | uniq > {o:excape_compids}`)
	excapeDBCompIDs.SetPathStatic("excape_compids", "dat/excapedb_compids.csv")
	excapeDBCompIDs.In("excapedb").Connect(dataExcapeDB)

	genRandSrcForDrugBankSelection := wf.NewProc("gen_randsrc_for_drugbank_selection", "dd if=/dev/urandom of={o:rand} bs=1024 count=1024") // HERE
	genRandSrcForDrugBankSelection.SetPathStatic("rand", "dat/randsrc_for_drugbank_selection.bin")

	// Approved/Withdrawn status in DrugBank is not mutually exclusive, so we need
	// to filter out the approved ones that are only approved, but NOT also
	// withdrawn
	extractUniquelyApproved := wf.NewProc("extract_uniquely_approved", `awk 'FNR==NR { withdr[$0]; next } !($1 in withdr)' {i:withdr} {i:approv} > {o:uniqapprov}`)
	extractUniquelyApproved.SetPathExtend("approv", "uniqapprov", ".uniq_appr.csv")
	extractUniquelyApproved.In("withdr").Connect(drugBankCompIDsWithdr.Out("compids"))
	extractUniquelyApproved.In("approv").Connect(drugBankCompIDsApprov.Out("compids"))

	// Filter out only the DrugBank compound IDs available in DrugBank
	drugBankCompIDsInExcapeDBCmd := `awk -F"," 'FNR==NR { edb[$1]; next } ($1 in edb) || ($2 in edb)' {i:excape_compids} {i:drugbank} > {o:out}`
	drugBankCompIDsInExcapeDBApprov := wf.NewProc("drugbank_compids_in_excapedb_approv", drugBankCompIDsInExcapeDBCmd)
	drugBankCompIDsInExcapeDBApprov.SetPathExtend("drugbank", "out", ".inexcapedb.csv")
	drugBankCompIDsInExcapeDBApprov.In("excape_compids").Connect(excapeDBCompIDs.Out("excape_compids"))
	drugBankCompIDsInExcapeDBApprov.In("drugbank").Connect(extractUniquelyApproved.Out("uniqapprov"))
	drugBankCompIDsInExcapeDBWithdr := wf.NewProc("drugbank_compids_in_excapedb_withdr", drugBankCompIDsInExcapeDBCmd)
	drugBankCompIDsInExcapeDBWithdr.SetPathExtend("drugbank", "out", ".inexcapedb.csv")
	drugBankCompIDsInExcapeDBWithdr.In("excape_compids").Connect(excapeDBCompIDs.Out("excape_compids"))
	drugBankCompIDsInExcapeDBWithdr.In("drugbank").Connect(drugBankCompIDsWithdr.Out("compids"))

	// Extract the approved compounds in DrugBank that we want to add to our set of
	// DrugBank compounds to remove from the dataset before training
	extractApprovedToAdd := wf.NewProc("extract_approved_to_add", `shuf --random-source={i:randsrc} -n $(awk 'END { print {p:nrcomp}-NR }' {i:withdr}) {i:approv} > {o:approved_to_add}`)
	extractApprovedToAdd.SetPathCustom("approved_to_add", func(t *sp.Task) string {
		return "dat/drugbank_compids_appr_to_add_for_tot_n" + t.Param("nrcomp") + ".csv"
	})
	extractApprovedToAdd.In("approv").Connect(drugBankCompIDsInExcapeDBApprov.Out("out"))
	extractApprovedToAdd.In("withdr").Connect(drugBankCompIDsInExcapeDBWithdr.Out("out"))
	extractApprovedToAdd.In("randsrc").Connect(genRandSrcForDrugBankSelection.Out("rand"))
	extractApprovedToAdd.ParamInPort("nrcomp").ConnectStr("1000")

	// Simply merge the withdrawn compounds from DrugBank, with the selected
	// approved ones, into one file
	mergeApprWithdr := wf.NewProc("merge_appr_withdr", "cat {i:approv} {i:withdr} | sort -V | uniq > {o:out}")
	mergeApprWithdr.SetPathStatic("out", "dat/drugbank_compids_to_remove.csv")
	mergeApprWithdr.In("approv").Connect(extractApprovedToAdd.Out("approved_to_add"))
	mergeApprWithdr.In("withdr").Connect(drugBankCompIDsInExcapeDBWithdr.Out("out"))

	// Merge the (sometimes) two comma-separated columns of compound IDs into one
	// column, so it can be used as a skip-list for filtering out the selected
	// DrugBank compounds in AWK later
	makeOneColumn := wf.NewProc("make_one_column", `cat {i:infile} | tr "," "\n" | sed '/^,$/d' | sed '/^$/d' | sort -V > {o:onecol}`)
	makeOneColumn.SetPathExtend("infile", "onecol", ".onecol.csv")
	makeOneColumn.In("infile").Connect(mergeApprWithdr.Out("out"))

	drugBankIdsCsvToTsv := wf.NewProc("drugbank_ids_csv_to_tsv", `cat {i:csv} | tr "," "\t" > {o:tsv}`)
	drugBankIdsCsvToTsv.SetPathReplace("csv", "tsv", ".csv", ".tsv")
	drugBankIdsCsvToTsv.In("csv").Connect(mergeApprWithdr.Out("out"))

	// extractGISA extracts a file with only Gene symbol, id (orig entry), SMILES,
	// and the Activity flag into a .tsv file, for easier subsequent parsing.
	// ATTENTION: The sorting order (Gene, SMILES, Activity) is super important,
	// for the following component, `removeConflicting` to function properly!
	extractGISA := wf.NewProc("extract_gene_id_smiles_activity", `awk -F "\t" '{ print $9 "\t" $2 "\t" $12 "\t" $4 }' {i:excapedb} | sort -uV -k 1,1 -k 3,3 -k 4,4 > {o:gene_id_smiles_activity}`)
	extractGISA.SetPathReplace("excapedb", "gene_id_smiles_activity", ".tsv", ".gisa.tsv")
	extractGISA.In("excapedb").Connect(dataExcapeDB)

	// removeConflicting prints the previous line, unless it has the same values on
	removeConflicting := wf.NewProc("remove_conflicting", `awk -F "\t" '((( prev1 != $1 ) && ( prev1 != "")) || (( prev3 != $3 ) && ( prev3 != "" ))) && !isconflicting[prev1,prev3] { print prev1 "\t" prev2 "\t" prev3 "\t" prev4 }
																( seen[$1,$3] > 0 ) && ( activity[$1,$3] != $4 ) { isconflicting[$1,$3] = true }
																{ seen[$1,$3]++; activity[$1,$3] = $4; prev1 = $1; prev2 = $2; prev3 = $3; prev4 = $4 }
																END { print }' \
															{i:gene_id_smiles_activity} > {o:gene_id_smiles_activity}`)
	removeConflicting.SetPathReplace("gene_id_smiles_activity", "gene_id_smiles_activity", ".tsv", ".dedup.tsv")
	removeConflicting.In("gene_id_smiles_activity").Connect(extractGISA.Out("gene_id_smiles_activity"))

	// Create process for subtracting the DrugBank compounds HERE
	remDrugBankComps := wf.NewProc("remove_drugbank_compounds", `awk 'FNR==NR { db[$1]; next } !($2 in db)' {i:compids_to_remove} {i:gisa} | sort -uV > {o:gisa_wo_drugbank}`)
	remDrugBankComps.SetPathStatic("gisa_wo_drugbank", "dat/excapedb.gisa_wo_drugbank.tsv")
	remDrugBankComps.In("compids_to_remove").Connect(makeOneColumn.Out("onecol"))
	remDrugBankComps.In("gisa").Connect(removeConflicting.Out("gene_id_smiles_activity"))

	// extractValidationRawdata prepares a data file for use in validation at the end of the workflow
	extractValidationRawdata := wf.NewProc("extract_validation_rawdata", `awk -F"\t" 'FNR==NR { cid[$1]; cbl[$2]; next } (( $2 in cid ) || ($2 in cbl )) { print }' {i:removed_compids} {i:gisa} | sort -uV > {o:drugbank_removed}`)
	extractValidationRawdata.In("removed_compids").Connect(drugBankIdsCsvToTsv.Out("tsv"))
	extractValidationRawdata.In("gisa").Connect(removeConflicting.Out("gene_id_smiles_activity"))
	extractValidationRawdata.SetPathExtend("gisa", "drugbank_removed", ".drugbank_removed.tsv")

	finalModelsSummary := NewFinalModelSummarizer(wf, "finalmodels_summary_creator", "res/final_models_summary.tsv", '\t')

	genRandomProcs := map[string]*sp.Process{}

	// We only do the fill run-set here (filling up for "small" datasets)
	runSets := []string{"fill"} // []string{"orig", "fill"}

	calibPlotPorts := []*sp.OutPort{}

	// --------------------------------
	// Set up gene-specific workflow branches
	// --------------------------------
	for _, geneUppercase := range geneSets[*geneSet] {
		geneLowerCase := str.ToLower(geneUppercase)
		uniqStrGene := geneLowerCase

		// extractTargetData extract all data for the specific target, into a separate file
		extractTargetData := wf.NewProc("extract_target_data_"+uniqStrGene, `awk 'END { print "smiles\tactivity" }' /dev/null > {o:target_data} && awk -F"\t" '$1 == "{p:gene}" { print $3"\t"$4 }' {i:raw_data} >> {o:target_data}`)
		extractTargetData.ParamInPort("gene").ConnectStr(geneUppercase)
		extractTargetData.SetPathStatic("target_data", fmt.Sprintf("dat/%s/%s.tsv", geneLowerCase, geneLowerCase))
		extractTargetData.In("raw_data").Connect(remDrugBankComps.Out("gisa_wo_drugbank"))
		if *runSlurm {
			extractTargetData.Prepend = "salloc -A snic2017-7-89 -n 4 -c 4 -t 1:00:00 -J scipipe_extract_" + geneLowerCase // SLURM string
		}

		for _, runSet := range runSets {
			uniqStrRunSet := uniqStrGene + "_" + runSet

			doFillUp := false
			if runSet == "fill" && strInSlice(geneUppercase, geneSets["bowes44min100percls_small"]) {
				doFillUp = true
			}

			countProcs := map[string]*sp.Process{}
			for i, replicate := range replicates {
				seed := i + 1 // Not sure if safe to use 0 as seed in CPSign, so ...(?)
				uniqStrRepl := uniqStrRunSet + "_" + replicate

				var assumedNonActive *sp.OutPort

				if doFillUp {
					sp.Audit.Printf("Filling up dataset with assumed negatives, for gene %s ...\n", geneUppercase)
					genRandomID := "create_random_bytes_" + replicate
					if _, ok := genRandomProcs[genRandomID]; !ok {
						genRandomProcs[genRandomID] = wf.NewProc(genRandomID, "dd if=/dev/urandom of={o:rand} bs=1048576 count=1024")
					}
					genRandomProcs[genRandomID].SetPathStatic("rand", "dat/"+replicate+"_random_bytes.bin")

					// Here we fill up TO the double amount of non-actives compared
					// to number of actives, by multiplying the number of actives
					// times two, and subtracting the number of existing
					// non-actices (See "A*2-N" in the AWK-script below).
					extractAssumedNonBinding := wf.NewProc("extract_assumed_n_"+uniqStrRepl, `
					let "fillup_lines_cnt = "$(awk -F"\t" '$2 == "A" { A += 1 } $2 == "N" { N += 1 } END { print A*2-N }' {i:targetdata}) \
					&& awk -F"\t" 'FNR==NR{target_smiles[$1]; next} ($1 != "{p:gene}") && !($3 in target_smiles) { print $3 "\tN" }' {i:targetdata} {i:rawdata} \
					| sort -uV \
					| shuf --random-source={i:randsrc} -n $fillup_lines_cnt > {o:assumed_n} # replicate:{p:replicate}`)
					extractAssumedNonBinding.SetPathCustom("assumed_n", func(t *sp.Task) string {
						gene := str.ToLower(t.Param("gene"))
						repl := t.Param("replicate")
						return "dat/" + gene + "/" + repl + "/" + gene + "." + repl + ".assumed_n.tsv"
					})
					extractAssumedNonBinding.In("rawdata").Connect(remDrugBankComps.Out("gisa_wo_drugbank"))
					extractAssumedNonBinding.In("targetdata").Connect(extractTargetData.Out("target_data"))
					extractAssumedNonBinding.ParamInPort("gene").ConnectStr(geneUppercase)
					extractAssumedNonBinding.ParamInPort("replicate").ConnectStr(replicate)
					extractAssumedNonBinding.In("randsrc").Connect(genRandomProcs[genRandomID].Out("rand"))
					assumedNonActive = extractAssumedNonBinding.Out("assumed_n")
				}

				if replicate == "r1" {
					catPart := `cat {i:targetdata}`
					if doFillUp {
						catPart = `cat {i:targetdata} {i:assumed_n}`
					}
					countProcs[uniqStrRunSet] = wf.NewProc("cnt_targetdata_rows_"+uniqStrRepl, catPart+` | awk '$2 == "A" { a += 1 } $2 == "N" { n += 1 } END { print a "\t" n }' > {o:count} # {p:runset} {p:gene} {p:replicate}`)
					countProcs[uniqStrRunSet].SetPathCustom("count", func(t *sp.Task) string {
						gene := str.ToLower(t.Param("gene"))
						repl := t.Param("replicate")
						rset := t.Param("runset")
						return "dat/" + gene + "/" + repl + "/" + rset + "/" + gene + "." + repl + "." + rset + ".cnt"
					})
					countProcs[uniqStrRunSet].In("targetdata").Connect(extractTargetData.Out("target_data"))
					if doFillUp {
						countProcs[uniqStrRunSet].In("assumed_n").Connect(assumedNonActive)
					}
					countProcs[uniqStrRunSet].ParamInPort("runset").ConnectStr(runSet)
					countProcs[uniqStrRunSet].ParamInPort("gene").ConnectStr(geneUppercase)
					countProcs[uniqStrRunSet].ParamInPort("replicate").ConnectStr(replicate)
				}

				// --------------------------------------------------------------------------------
				// Pre-compute step
				// --------------------------------------------------------------------------------
				cpSignPrecompCmd := `java -jar ` + cpSignPath + ` precompute \
									--license ../../bin/cpsign.lic \
									--cptype 1 \
									--trainfile {i:traindata} \
									--response-name activity \
									--labels A, N \
									--model-out {o:precomp} \
									--model-name "` + geneUppercase + `" \
									--logfile {o:logfile}`
				if doFillUp {
					cpSignPrecompCmd += ` \
									--proper-trainfile {i:propertraindata}`
				}
				cpSignPrecompCmd += ` # {p:gene} {p:runset} {p:replicate}`
				cpSignPrecomp := wf.NewProc("cpsign_precomp_"+uniqStrRepl, cpSignPrecompCmd)
				cpSignPrecomp.In("traindata").Connect(extractTargetData.Out("target_data"))
				if doFillUp {
					cpSignPrecomp.In("propertraindata").Connect(assumedNonActive)
				}
				cpSignPrecomp.ParamInPort("gene").ConnectStr(geneLowerCase)
				cpSignPrecomp.ParamInPort("replicate").ConnectStr(replicate)
				cpSignPrecomp.ParamInPort("runset").ConnectStr(runSet)
				precompPathFunc := func(t *sp.Task) string {
					gene := t.Param("gene")
					repl := t.Param("replicate")
					rset := t.Param("runset")
					return "dat/" + gene + "/" + repl + "/" + rset + "/" + gene + "." + repl + "." + rset + ".precomp"
				}
				cpSignPrecomp.SetPathCustom("precomp", precompPathFunc)
				cpSignPrecomp.SetPathCustom("logfile", func(t *sp.Task) string {
					return precompPathFunc(t) + ".cpsign.log"
				})
				if *runSlurm {
					cpSignPrecomp.Prepend = "salloc -A snic2017-7-89 -n 4 -c 4 -t 1-00:00:00 -J precmp_" + geneLowerCase // SLURM string
				}

				// --------------------------------------------------------------------------------
				// Optimize cost/gamma-step
				// --------------------------------------------------------------------------------
				includeGamma := false // For liblinear
				summarize := NewSummarizeCostGammaPerf(wf,
					"summarize_cost_gamma_perf_"+uniqStrRepl,
					"dat/"+runSet+"/"+geneLowerCase+"/"+replicate+"/"+geneLowerCase+"_cost_gamma_perf_stats.tsv",
					includeGamma)

				for _, cost := range costsPerTarget[geneUppercase] {
					uniqStrCost := uniqStrRepl + "_" + cost
					// If Liblinear
					evalCostCmd := `java -jar ` + cpSignPath + ` crossvalidate \
									--license ../../bin/cpsign.lic \
									--seed {p:seed} \
									--cptype 1 \
									--trainfile {i:traindata} \
									--response-name activity \
									--impl liblinear \
									--labels A, N \
									--nr-models {p:nrmdl} \
									--cost {p:cost} \
									--cv-folds {p:cvfolds} \
									--output-format json \
									--logfile {o:logfile}`
					if doFillUp {
						evalCostCmd += ` \
									--proper-trainfile {i:propertraindata}`
					}
					evalCostCmd += ` \
									--confidences "{p:confidences}" | grep -P "^\[" > {o:stats} # {p:gene} {p:runset} {p:replicate}`
					evalCost := wf.NewProc("crossval_"+uniqStrCost, evalCostCmd)
					evalCostStatsPathFunc := func(t *sp.Task) string {
						cost, err := strconv.ParseInt(t.Param("cost"), 10, 0)
						sp.Check(err)
						gene := str.ToLower(t.Param("gene"))
						repl := t.Param("replicate")
						rset := t.Param("runset")
						return filepath.Dir(t.InPath("traindata")) + "/" + repl + "/" + rset + "/" + fmt.Sprintf("%s.%s.%s.liblin_c%03d", gene, repl, rset, cost) + ".cvstats.json"
					}
					evalCost.SetPathCustom("stats", evalCostStatsPathFunc)
					evalCost.SetPathCustom("logfile", func(t *sp.Task) string {
						return evalCostStatsPathFunc(t) + ".cpsign.log"
					})
					if doFillUp {
						evalCost.In("propertraindata").Connect(assumedNonActive)
					}
					evalCost.In("traindata").Connect(extractTargetData.Out("target_data"))
					evalCost.ParamInPort("seed").ConnectStr(fmt.Sprintf("%d", seed))
					evalCost.ParamInPort("nrmdl").ConnectStr("10")
					evalCost.ParamInPort("cvfolds").ConnectStr("10")
					evalCost.ParamInPort("confidences").ConnectStr("0.05, 0.1, 0.15, 0.2, 0.25, 0.3, 0.35, 0.4, 0.45, 0.5, 0.55, 0.6, 0.65, 0.7, 0.75, 0.8, 0.85, 0.9, 0.95")
					evalCost.ParamInPort("gene").ConnectStr(geneUppercase)
					evalCost.ParamInPort("runset").ConnectStr(runSet)
					evalCost.ParamInPort("replicate").ConnectStr(replicate)
					evalCost.ParamInPort("cost").ConnectStr(cost)
					if *runSlurm {
						evalCost.Prepend = "salloc -A snic2017-7-89 -n 4 -c 4 -t 1-00:00:00 -J evalcg_" + uniqStrCost // SLURM string
					}

					extractCalibrationData := wf.NewProc("extract_calibration_data_"+uniqStrCost, "# {i:cvstats} {o:tsv}")
					extractCalibrationData.SetPathExtend("cvstats", "tsv", ".calibration.tsv")
					extractCalibrationData.In("cvstats").Connect(evalCost.Out("stats"))
					extractCalibrationData.CustomExecute = func(t *sp.Task) {
						tsvFh := t.OutIP("tsv").OpenWriteTemp()
						defer tsvFh.Close()

						tsvWrt := csv.NewWriter(tsvFh)
						tsvWrt.Comma = '\t'

						cvStatsRecords := &[]cpSignCrossValOutput{}
						t.InIP("cvstats").UnMarshalJSON(cvStatsRecords)

						tsvWrt.Write([]string{"confidence", "accuracty"})
						for _, crossValOut := range *cvStatsRecords {
							confidence := fmt.Sprintf("%.3f", crossValOut.Confidence)
							accuracy := fmt.Sprintf("%.3f", crossValOut.Accuracy)
							tsvWrt.Write([]string{confidence, accuracy})
						}
						tsvWrt.Flush()
					}

					plotCalibrationData := wf.NewProc("plot_calibration_data_"+uniqStrCost, "Rscript bin/plot_calibration.r -i {i:tsv} -o {o:pdf} -f pdf -g {p:gene}")
					plotCalibrationData.SetPathExtend("tsv", "pdf", ".pdf")
					plotCalibrationData.ParamInPort("gene").ConnectStr(geneUppercase)
					plotCalibrationData.In("tsv").Connect(extractCalibrationData.Out("tsv"))
					calibPlotPorts = append(calibPlotPorts, plotCalibrationData.Out("pdf"))

					extractCostGammaStats := spc.NewMapToKeys(wf, "extract_cgstats_"+uniqStrCost, func(ip *sp.FileIP) map[string]string {
						newKeys := map[string]string{}
						crossValOuts := &[]cpSignCrossValOutput{}
						ip.UnMarshalJSON(crossValOuts)
						for _, crossValOut := range *crossValOuts {
							if diff := math.Abs(crossValOut.Confidence - 0.9); diff < 0.001 {
								newKeys["confidence"] = fmt.Sprintf("%.3f", crossValOut.Confidence)
								newKeys["accuracy"] = fmt.Sprintf("%.3f", crossValOut.Accuracy)
								newKeys["efficiency"] = fmt.Sprintf("%.3f", crossValOut.Efficiency)
								newKeys["class_confidence"] = fmt.Sprintf("%.3f", crossValOut.ClassConfidence)
								newKeys["class_credibility"] = fmt.Sprintf("%.3f", crossValOut.ClassCredibility)
								newKeys["obsfuzz_active"] = fmt.Sprintf("%.3f", crossValOut.ObservedFuzziness.Active)
								newKeys["obsfuzz_nonactive"] = fmt.Sprintf("%.3f", crossValOut.ObservedFuzziness.Nonactive)
								newKeys["obsfuzz_overall"] = fmt.Sprintf("%.3f", crossValOut.ObservedFuzziness.Overall)
							}
						}
						return newKeys
					})
					extractCostGammaStats.In().Connect(evalCost.Out("stats"))

					summarize.In().Connect(extractCostGammaStats.Out())
				} // end for cost

				selectBest := NewBestCostGamma(wf,
					"select_best_cost_gamma_"+uniqStrRepl,
					'\t',
					false, includeGamma)
				selectBest.InCSVFile().Connect(summarize.OutStats())

				// --------------------------------------------------------------------------------
				// Train step
				// --------------------------------------------------------------------------------
				cpSignTrain := wf.NewProc("cpsign_train_"+uniqStrRepl,
					`java -jar `+cpSignPath+` train \
									--license ../../bin/cpsign.lic \
									--seed {p:seed} \
									--cptype 1 \
									--modelfile {i:model} \
									--labels A, N \
									--impl liblinear \
									--nr-models {p:nrmdl} \
									--cost {p:cost} \
									--percentilesfile {i:percentilesfile} \
									--percentiles {p:nrpercentiles} \
									--model-out {o:model} \
									--logfile {o:logfile} \
									--model-name "{p:gene}" # {p:runset} {p:replicate} Accuracy: {p:accuracy} Efficiency: {p:efficiency} Class-Equalized Observed Fuzziness: {p:obsfuzz_classavg} Observed Fuzziness (Overall): {p:obsfuzz_overall} Observed Fuzziness (Active class): {p:obsfuzz_active} Observed Fuzziness (Non-active class): {p:obsfuzz_nonactive} Class Confidence: {p:class_confidence} Class Credibility: {p:class_credibility}`)
				cpSignTrain.In("model").Connect(cpSignPrecomp.Out("precomp"))
				cpSignTrain.In("percentilesfile").Connect(extractTargetData.Out("target_data"))
				cpSignTrain.ParamInPort("seed").ConnectStr(fmt.Sprintf("%d", seed))
				cpSignTrain.ParamInPort("nrmdl").ConnectStr("10")
				cpSignTrain.ParamInPort("gene").ConnectStr(geneUppercase)
				cpSignTrain.ParamInPort("replicate").ConnectStr(replicate)
				cpSignTrain.ParamInPort("runset").ConnectStr(runSet)
				cpSignTrain.ParamInPort("accuracy").Connect(selectBest.OutBestAccuracy())
				cpSignTrain.ParamInPort("efficiency").Connect(selectBest.OutBestEfficiency())
				cpSignTrain.ParamInPort("obsfuzz_classavg").Connect(selectBest.OutBestObsFuzzClassAvg())
				cpSignTrain.ParamInPort("obsfuzz_overall").Connect(selectBest.OutBestObsFuzzOverall())
				cpSignTrain.ParamInPort("obsfuzz_active").Connect(selectBest.OutBestObsFuzzActive())
				cpSignTrain.ParamInPort("obsfuzz_nonactive").Connect(selectBest.OutBestObsFuzzNonactive())
				cpSignTrain.ParamInPort("class_confidence").Connect(selectBest.OutBestClassConfidence())
				cpSignTrain.ParamInPort("class_credibility").Connect(selectBest.OutBestClassCredibility())
				cpSignTrain.ParamInPort("cost").Connect(selectBest.OutBestCost())
				cpSignTrain.ParamInPort("nrpercentiles").ConnectStr("200") // Reasonable number according to staffan
				cpSignTrainModelPathFunc := func(t *sp.Task) string {
					return fmt.Sprintf("dat/final_models/%s/%s/%s/%s.%s.%s.%s_c%s_nrmdl%s.mdl.jar",
						str.ToLower(t.Param("gene")),
						t.Param("replicate"),
						t.Param("runset"),
						str.ToLower(t.Param("gene")),
						t.Param("replicate"),
						t.Param("runset"),
						"liblin",
						t.Param("cost"),
						t.Param("nrmdl"))
				}
				cpSignTrain.SetPathCustom("model", cpSignTrainModelPathFunc)
				cpSignTrain.SetPathCustom("logfile", func(t *sp.Task) string {
					return cpSignTrainModelPathFunc(t) + ".cpsign.log"
				})
				if *runSlurm {
					cpSignTrain.Prepend = "salloc -A snic2017-7-89 -n 4 -c 4 -t 1-00:00:00 -J train_" + uniqStrRepl // SLURM string
				}

				embedAuditLog := NewEmbedAuditLogInJar(wf, "embed_auditlog_"+uniqStrRepl)
				embedAuditLog.InJarFile().Connect(cpSignTrain.Out("model"))

				finalModelsSummary.InModel().Connect(cpSignTrain.Out("model"))

				// ------------------------------------------
				// Validate excluded DrugBank compounds (compare predicted and actual values)
				// ------------------------------------------
				// gisa: gene, id, smiles, activity. sa: smiles, activity

				// extractTargetValidationData -----------------------------------
				extractTargetValidationData := wf.NewProc("extract_target_validation_data_"+uniqStrRepl, `awk '
					( $1 == "`+geneUppercase+`" ) { print $2 "\t" $3 "\t" $4 }' \
				{i:raw} > {o:tgt} # {p:gene} {p:replicate} {p:runset}`)
				extractTargetValidationData.SetPathCustom("tgt", func(t *sp.Task) string {
					uniqStrReplRunset := t.Param("gene") + "." + t.Param("replicate") + "." + t.Param("runset")
					return "dat/validate/" + uniqStrReplRunset + "/" + uniqStrReplRunset + ".validation_data.tsv"
				})
				extractTargetValidationData.In("raw").Connect(extractValidationRawdata.Out("drugbank_removed")) // HERE
				extractTargetValidationData.ParamInPort("gene").ConnectStr(geneLowerCase)
				extractTargetValidationData.ParamInPort("replicate").ConnectStr(replicate)
				extractTargetValidationData.ParamInPort("runset").ConnectStr(runSet)

				// dedupTargetValData --------------------------------------------
				dedupTargetValData := wf.NewProc("dedup_target_validation_data_"+uniqStrRepl, `awk '
					FNR == NR { ids[$1] = $2 "\t" $3; next }
					( $1 in ids ) { print ids[$1] }
					(!( $1 in ids ) && ( $2 in ids )) { print ids[$2] }' \
				{i:target_val_data} {i:drugbank_compids} > {o:dedup}`)
				dedupTargetValData.SetPathExtend("target_val_data", "dedup", ".dedup.tsv")
				dedupTargetValData.In("target_val_data").Connect(extractTargetValidationData.Out("tgt"))
				dedupTargetValData.In("drugbank_compids").Connect(drugBankIdsCsvToTsv.Out("tsv"))

				// validateDrugBank ----------------------------------------------
				validateDrugBank := wf.NewProc("validate_drugbank_"+uniqStrRepl, `java -jar `+cpSignPath+` validate \
									--license ../../bin/cpsign.lic \
									--cptype 1 \
									--modelfile {i:model} \
									--predictfile {i:smiles} \
									--validation-property activity \
									--confidences {p:confidences} \
									--output-format json \
									--logfile {o:log} \
									--output {o:json} # {p:gene} {p:replicate} {p:runset}`)
				validateDrugBankJSONPathFunc := func(t *sp.Task) string {
					uniqStrReplRunset := t.Param("gene") + "." + t.Param("replicate") + "." + t.Param("runset")
					return "dat/validate/" + uniqStrReplRunset + "/" + uniqStrReplRunset + ".validate_drugbank_1000.json"
				}
				validateDrugBank.SetPathCustom("json", validateDrugBankJSONPathFunc)
				validateDrugBank.SetPathCustom("log", func(t *sp.Task) string {
					return validateDrugBankJSONPathFunc(t) + ".cpsign.log"
				})
				validateDrugBank.In("model").Connect(cpSignTrain.Out("model"))
				validateDrugBank.In("smiles").Connect(dedupTargetValData.Out("dedup")) // Create target specific data file
				validateDrugBank.ParamInPort("gene").ConnectStr(geneLowerCase)
				validateDrugBank.ParamInPort("replicate").ConnectStr(replicate)
				validateDrugBank.ParamInPort("runset").ConnectStr(runSet)
				validateDrugBank.ParamInPort("confidences").ConnectStr("0.8, 0.9")

			} // end: for replicate
			finalModelsSummary.InTargetDataCount().Connect(countProcs[uniqStrRunSet].Out("count"))
		} // end: runset
	} // end: for gene

	sts := spc.NewStreamToSubStream(wf, "merge_plots_stream_to_substream")
	for _, calibPlotPort := range calibPlotPorts {
		sts.In().Connect(calibPlotPort)
	}
	mergeCalibPlots := wf.NewProc("merge_calibration_plots", "montage -tile 7x -geometry +2+2 {i:plots:r: } {o:merged}")
	mergeCalibPlots.SetPathStatic("merged", "dat/calibration_plots.png")
	mergeCalibPlots.In("plots").Connect(sts.OutSubStream())

	sortSummaryOnDataSize := wf.NewProc("sort_summary", "head -n 1 {i:summary} > {o:sorted} && tail -n +2 {i:summary} | sort -k 17n,17 -k 2,2 -k 3r,3 >> {o:sorted}")
	sortSummaryOnDataSize.SetPathReplace("summary", "sorted", ".tsv", ".sorted.tsv")
	sortSummaryOnDataSize.In("summary").Connect(finalModelsSummary.OutSummary())

	for _, runSet := range runSets {
		plotSummary := wf.NewProc("plot_summary_"+runSet, "Rscript bin/plot_summary.r -i {i:summary} -o {o:plot} -f pdf # gene:{i:gene_smiles_activity} runset:{p:runset}")
		plotSummary.SetPathExtend("summary", "plot", "."+runSet+".pdf")
		plotSummary.In("summary").Connect(sortSummaryOnDataSize.Out("sorted"))
		plotSummary.In("gene_smiles_activity").Connect(remDrugBankComps.Out("gisa_wo_drugbank"))
		plotSummary.ParamInPort("runset").ConnectStr(runSet)
	}

	// --------------------------------
	// Run the pipeline!
	// --------------------------------
	if *graph {
		wf.PlotGraph("wo_drugbank_wf.dot", true, true)
	} else {
		wf.RunToRegex(*procsRegex)
	}
}

// --------------------------------------------------------------------------------
// JSON types
// --------------------------------------------------------------------------------
// JSON output of cpSign crossvalidate
// {
//     "classConfidence": 0.855,
//     "observedFuzziness": {
//         "A": 0.253,
//         "N": 0.207,
//         "overall": 0.231
//     },
//     "accuracy": 0.917,
//     "efficiency": 0.333,
//     "classCredibility": 0.631
// }
// --------------------------------------------------------------------------------

type cpSignCrossValOutput struct {
	Efficiency        float64                 `json:"efficiency"`
	Confidence        float64                 `json:"confidence"`
	ClassCredibility  float64                 `json:"classCredibility"`
	Accuracy          float64                 `json:"accuracy"`
	ClassConfidence   float64                 `json:"classConfidence"`
	ObservedFuzziness cpSignObservedFuzziness `json:"observedFuzziness"`
}

type cpSignObservedFuzziness struct {
	Active    float64 `json:"A"`
	Nonactive float64 `json:"N"`
	Overall   float64 `json:"overall"`
}

func strInSlice(searchStr string, strings []string) bool {
	for _, str := range strings {
		if searchStr == str {
			return true
		}
	}
	return false
}
