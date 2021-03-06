// Workflow written in SciPipe.  // For more information about SciPipe, see: http://scipipe.org
package main

import (
	"flag"
	"fmt"
	"runtime"
	"strconv"
	str "strings"

	sp "github.com/scipipe/scipipe"
	spc "github.com/scipipe/scipipe/components"
)

var (
	maxTasks = flag.Int("maxtasks", 4, "Max number of local cores to use")
	threads  = flag.Int("threads", 1, "Number of threads that Go is allowed to start")
	geneSet  = flag.String("geneset", "smallest1", "Gene set to use (one of smallest1, smallest3, smallest4, bowes44)")
	runSlurm = flag.Bool("slurm", false, "Start computationally heavy jobs via SLURM")
	debug    = flag.Bool("debug", false, "Increase logging level to include DEBUG messages")

	cpSignPath = "../../bin/cpsign-0.6.3.jar"
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
	costVals = []string{
		"1",
		"10",
		"100",
	}
	gammaVals = []string{
		"0.1",
		"0.01",
		"0.001",
	}
	replicates = []string{
		"r1", "r2", "r3",
	}
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
		for n, _ := range geneSets {
			names = append(names, n)
		}
		sp.Error.Fatalf("Incorrect gene set %s specified! Only allowed values are: %s\n", *geneSet, str.Join(names, ", "))
	}
	runtime.GOMAXPROCS(*threads)

	// --------------------------------
	// Show startup messages
	// --------------------------------
	sp.Info.Printf("Using max %d OS threads to schedule max %d tasks\n", *threads, *maxTasks)
	sp.Info.Printf("Starting workflow for %s geneset\n", *geneSet)

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
	//unPackDB.Prepend = "salloc -A snic2017-7-89 -n 2 -t 8:00:00 -J unpack_excapedb"

	finalModelsSummary := NewFinalModelSummarizer(wf, "finalmodels_summary_creator", "res/final_models_summary.tsv", '\t')
	// --------------------------------
	// Set up gene-specific workflow branches
	// --------------------------------
	for _, gene := range geneSets[*geneSet] {
		geneLC := str.ToLower(gene)
		uniq_gene := geneLC

		// --------------------------------------------------------------------------------
		// Extract target data step
		// --------------------------------------------------------------------------------
		extractTargetData := wf.NewProc("extract_target_data_"+uniq_gene, `awk -F"\t" '$9 == "{p:gene}" { print $12"\t"$4 }' {i:raw_data} > {o:target_data}`)
		extractTargetData.ParamPort("gene").ConnectStr(gene)
		extractTargetData.SetPathStatic("target_data", fmt.Sprintf("dat/%s/%s.tsv", geneLC, geneLC))
		extractTargetData.In("raw_data").Connect(unPackDB.Out("unxzed"))
		if *runSlurm {
			extractTargetData.Prepend = "salloc -A snic2017-7-89 -n 4 -c 4 -t 1:00:00 -J scipipe_cnt_comp_" + geneLC // SLURM string
		}

		countTargetDataRows := wf.NewProc("cnt_targetdata_rows_"+uniq_gene, `awk '$2 == "A" { a += 1 } $2 == "N" { n += 1 } END { print a "\t" n }' {i:targetdata} > {o:count} # {p:gene}`)
		countTargetDataRows.SetPathExtend("targetdata", "count", ".count")
		countTargetDataRows.In("targetdata").Connect(extractTargetData.Out("target_data"))
		countTargetDataRows.ParamPort("gene").ConnectStr(gene)

		// --------------------------------------------------------------------------------
		// Pre-compute step
		// --------------------------------------------------------------------------------
		cpSignPrecomp := wf.NewProc("cpsign_precomp_"+uniq_gene,
			`java -jar `+cpSignPath+` precompute \
									--license ../../bin/cpsign.lic \
									--cptype 1 \
									--trainfile {i:traindata} \
									--labels A, N \
									--model-out {o:precomp} \
									--model-name "`+gene+` target profile"`)
		cpSignPrecomp.In("traindata").Connect(extractTargetData.Out("target_data"))
		cpSignPrecomp.SetPathExtend("traindata", "precomp", ".precomp")
		if *runSlurm {
			cpSignPrecomp.Prepend = "salloc -A snic2017-7-89 -n 4 -c 4 -t 1-00:00:00 -J precmp_" + geneLC // SLURM string
		}

		for _, replicate := range replicates {
			uniq_repl := uniq_gene + "_" + replicate

			// --------------------------------------------------------------------------------
			// Optimize cost/gamma-step
			// --------------------------------------------------------------------------------
			includeGamma := false // For liblinear
			summarize := NewSummarizeCostGammaPerf(wf,
				"summarize_cost_gamma_perf_"+uniq_repl,
				"dat/"+geneLC+"/"+replicate+"/"+geneLC+"_cost_gamma_perf_stats.tsv",
				includeGamma)

			for _, cost := range costVals {
				uniq_cost := uniq_repl + "_" + cost
				// If Liblinear
				evalCost := wf.NewProc("crossval_"+uniq_cost, `java -jar `+cpSignPath+` crossvalidate \
									--license ../../bin/cpsign.lic \
									--cptype 1 \
									--trainfile {i:traindata} \
									--impl liblinear \
									--labels A, N \
									--nr-models {p:nrmdl} \
									--cost {p:cost} \
									--cv-folds {p:cvfolds} \
									--output-format json \
									--confidence {p:confidence} | grep -P "^{" > {o:stats} # {p:gene} {p:replicate}`)
				evalCost.SetPathCustom("stats", func(t *sp.SciTask) string {
					c, err := strconv.ParseInt(t.Param("cost"), 10, 0)
					geneLC := str.ToLower(t.Param("gene"))
					sp.CheckErr(err)
					return str.Replace(t.InPath("traindata"), geneLC+".tsv", t.Param("replicate")+"/"+geneLC+".tsv", 1) + fmt.Sprintf(".liblin_c%03d", c) + "_crossval_stats.json"
				})
				evalCost.In("traindata").Connect(extractTargetData.Out("target_data"))
				evalCost.ParamPort("nrmdl").ConnectStr("10")
				evalCost.ParamPort("cvfolds").ConnectStr("10")
				evalCost.ParamPort("confidence").ConnectStr("0.9")
				evalCost.ParamPort("gene").ConnectStr(gene)
				evalCost.ParamPort("replicate").ConnectStr(replicate)
				evalCost.ParamPort("cost").ConnectStr(cost)
				if *runSlurm {
					evalCost.Prepend = "salloc -A snic2017-7-89 -n 4 -c 4 -t 1-00:00:00 -J evalcg_" + uniq_cost // SLURM string
				}

				extractCostGammaStats := spc.NewMapToKeys(wf, "extract_cgstats_"+uniq_cost, func(ip *sp.InformationPacket) map[string]string {
					crossValOut := &cpSignCrossValOutput{}
					ip.UnMarshalJson(crossValOut)
					newKeys := map[string]string{}
					newKeys["validity"] = fmt.Sprintf("%.3f", crossValOut.Validity)
					newKeys["efficiency"] = fmt.Sprintf("%.3f", crossValOut.Efficiency)
					newKeys["class_confidence"] = fmt.Sprintf("%.3f", crossValOut.ClassConfidence)
					newKeys["class_credibility"] = fmt.Sprintf("%.3f", crossValOut.ClassCredibility)
					newKeys["obsfuzz_active"] = fmt.Sprintf("%.3f", crossValOut.ObservedFuzziness.Active)
					newKeys["obsfuzz_nonactive"] = fmt.Sprintf("%.3f", crossValOut.ObservedFuzziness.Nonactive)
					newKeys["obsfuzz_overall"] = fmt.Sprintf("%.3f", crossValOut.ObservedFuzziness.Overall)
					return newKeys
				})
				extractCostGammaStats.In.Connect(evalCost.Out("stats"))

				summarize.In.Connect(extractCostGammaStats.Out)
			} // end for cost

			// TODO: Let select best operate directly on the stream of IPs, not
			// via the summarize component, so that we can retain the keys in
			// the IP!
			selectBest := NewBestCostGamma(wf,
				"select_best_cost_gamma_"+uniq_repl,
				'\t',
				false,
				includeGamma)
			selectBest.InCSVFile.Connect(summarize.OutStats)
			// --------------------------------------------------------------------------------
			// Train step
			// --------------------------------------------------------------------------------
			cpSignTrain := wf.NewProc("cpsign_train_"+uniq_repl,
				`java -jar `+cpSignPath+` train \
									--license ../../bin/cpsign.lic \
									--cptype 1 \
									--modelfile {i:model} \
									--labels A, N \
									--impl liblinear \
									--nr-models {p:nrmdl} \
									--cost {p:cost} \
									--model-out {o:model} \
									--model-name "{p:gene} target profile" # {p:replicate} Validity: {p:validity} Efficiency: {p:efficiency} Class-Equalized Observed Fuzziness: {p:obsfuzz_classavg} Observed Fuzziness (Overall): {p:obsfuzz_overall} Observed Fuzziness (Active class): {p:obsfuzz_active} Observed Fuzziness (Non-active class): {p:obsfuzz_nonactive} Class Confidence: {p:class_confidence} Class Credibility: {p:class_credibility}`)
			cpSignTrain.In("model").Connect(cpSignPrecomp.Out("precomp"))
			cpSignTrain.ParamPort("nrmdl").ConnectStr("10")
			cpSignTrain.ParamPort("gene").ConnectStr(gene)
			cpSignTrain.ParamPort("replicate").ConnectStr(replicate)
			cpSignTrain.ParamPort("validity").Connect(selectBest.OutBestValidity)
			cpSignTrain.ParamPort("efficiency").Connect(selectBest.OutBestEfficiency)
			cpSignTrain.ParamPort("obsfuzz_classavg").Connect(selectBest.OutBestObsFuzzClassAvg)
			cpSignTrain.ParamPort("obsfuzz_overall").Connect(selectBest.OutBestObsFuzzOverall)
			cpSignTrain.ParamPort("obsfuzz_active").Connect(selectBest.OutBestObsFuzzActive)
			cpSignTrain.ParamPort("obsfuzz_nonactive").Connect(selectBest.OutBestObsFuzzNonactive)
			cpSignTrain.ParamPort("class_confidence").Connect(selectBest.OutBestClassConfidence)
			cpSignTrain.ParamPort("class_credibility").Connect(selectBest.OutBestClassCredibility)
			cpSignTrain.ParamPort("cost").Connect(selectBest.OutBestCost)
			cpSignTrain.SetPathCustom("model", func(t *sp.SciTask) string {
				return fmt.Sprintf("dat/final_models/%s/%s_c%s_nrmdl%s_%s.mdl",
					str.ToLower(t.Param("gene")),
					"liblin",
					t.Param("cost"),
					t.Param("nrmdl"),
					t.Param("replicate"))
			})
			if *runSlurm {
				cpSignTrain.Prepend = "salloc -A snic2017-7-89 -n 4 -c 4 -t 1-00:00:00 -J train_" + uniq_repl // SLURM string
			}

			finalModelsSummary.InModel.Connect(cpSignTrain.Out("model"))
			finalModelsSummary.InTargetDataCount.Connect(countTargetDataRows.Out("count"))
		} // end for replicate
	} // end for gene

	sortSummaryOnDataSize := wf.NewProc("sort_summary", "head -n 1 {i:summary} > {o:sorted} && tail -n +2 {i:summary} | sort -nk 16 >> {o:sorted}")
	sortSummaryOnDataSize.SetPathReplace("summary", "sorted", ".tsv", ".sorted.tsv")
	sortSummaryOnDataSize.In("summary").Connect(finalModelsSummary.OutSummary)

	plotSummary := wf.NewProc("plot_summary", "Rscript bin/plot_summary.r -i {i:summary} -o {o:plot} -f png")
	plotSummary.SetPathExtend("summary", "plot", ".plot.png")
	plotSummary.In("summary").Connect(sortSummaryOnDataSize.Out("sorted"))

	wf.ConnectLast(plotSummary.Out("plot"))

	// --------------------------------
	// Run the pipeline!
	// --------------------------------
	wf.Run()
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
//     "validity": 0.917,
//     "efficiency": 0.333,
//     "classCredibility": 0.631
// }
// --------------------------------------------------------------------------------

type cpSignCrossValOutput struct {
	ClassConfidence   float64                 `json:"classConfidence"`
	ObservedFuzziness cpSignObservedFuzziness `json:"observedFuzziness"`
	Validity          float64                 `json:"validity"`
	Efficiency        float64                 `json:"efficiency"`
	ClassCredibility  float64                 `json:"classCredibility"`
}

type cpSignObservedFuzziness struct {
	Active    float64 `json:"A"`
	Nonactive float64 `json:"N"`
	Overall   float64 `json:"overall"`
}
