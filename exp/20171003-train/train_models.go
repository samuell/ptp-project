// Workflow written in SciPipe.
// For more information about SciPipe, see: http://scipipe.org
package main

import (
	"bytes"
	"encoding/csv"
	"flag"
	"fmt"
	sp "github.com/scipipe/scipipe"
	"io/ioutil"
	"regexp"
	"strconv"
	str "strings"
)

var (
	maxCores = flag.Int("maxcores", 4, "Max number of local cores to use")
	geneSet  = flag.String("geneset", "smallest1", "Gene set to use (one of smallest1, smallest3, smallest4, bowes44)")
	runSlurm = flag.Bool("slurm", false, "Start computationally heavy jobs via SLURM")

	cpSignPath = "../../bin/cpsign-0.6.2.jar"
	geneSets   = map[string][]string{
		"bowes44": []string{
			// Not available in dataset: "CHRNA1", Not available in dataset:
			// "KCNE1". Instead we use MinK1 as they both share the same alias
			// "MinK", and also confirmed by Wes to be the same.
			"ADORA2A", "ADRA1A", "ADRA2A", "ADRB1", "ADRB2",
			"CNR1", "CNR2", "CCKAR", "DRD1", "DRD2",
			"EDNRA", "HRH1", "HRH2", "OPRD1", "OPRK1",
			"OPRM1", "CHRM1", "CHRM2", "CHRM3", "HTR1A",
			"HTR1B", "HTR2A", "HTR2B", "AVPR1A", "CHRNA4",
			"CACNA1C", "GABRA1", "KCNH2", "KCNQ1", "MINK1",
			"GRIN1", "HTR3A", "SCN5A", "ACHE", "PTGS1",
			"PTGS2", "MAOA", "PDE3A", "PDE4D", "LCK",
			"SLC6A3", "SLC6A2", "SLC6A4", "AR", "NR3C1",
		},
		"smallest1": []string{
			"GABRA1",
		},
		"smallest3": []string{
			"GABRA1",
			"CACNA1C",
			"CHRNA4",
		},
		"smallest4": []string{
			"GABRA1",
			"CACNA1C",
			"CHRNA4",
			"PDE3A",
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
)

func main() {
	sp.InitLogAudit()
	flag.Parse()

	sp.Info.Printf("Using max %d cores\n", *maxCores)
	sp.Info.Printf("Starting workflow for %s geneset\n", *geneSet)

	//sp.InitLogDebug()
	wf := sp.NewWorkflow("train_models", *maxCores)

	// --------------------------------
	// Initialize processes and add to runner
	// --------------------------------
	dbFileName := "pubchem.chembl.dataset4publication_inchi_smiles.tsv.xz"
	dlExcapeDB := wf.NewProc("dlDB", fmt.Sprintf("wget https://zenodo.org/record/173258/files/%s -O {o:excapexz}", dbFileName))
	dlExcapeDB.SetPathStatic("excapexz", "../../raw/"+dbFileName)

	unPackDB := wf.NewProc("unPackDB", "xzcat {i:xzfile} > {o:unxzed}")
	unPackDB.SetPathReplace("xzfile", "unxzed", ".xz", "")
	unPackDB.In("xzfile").Connect(dlExcapeDB.Out("excapexz"))
	//unPackDB.Prepend = "salloc -A snic2017-7-89 -n 2 -t 8:00:00 -J unpack_excapedb"

	// --------------------------------
	// Set up gene-specific workflow branches
	// --------------------------------
	for _, gene := range geneSets[*geneSet] {
		geneLC := str.ToLower(gene)

		// --------------------------------------------------------------------------------
		// Extract target data step
		// --------------------------------------------------------------------------------
		procName := "extract_target_data_" + geneLC
		extractTargetData := wf.NewProc(procName, fmt.Sprintf(`awk -F"\t" '$9 == "%s" { print $12"\t"$4 }' {i:raw_data} > {o:target_data}`, gene))
		extractTargetData.SetPathStatic("target_data", fmt.Sprintf("dat/%s/%s.tsv", geneLC, geneLC))
		extractTargetData.In("raw_data").Connect(unPackDB.Out("unxzed"))
		if *runSlurm {
			extractTargetData.Prepend = "salloc -A snic2017-7-89 -n 4 -t 1:00:00 -J scipipe_cnt_comp_" + geneLC + " srun " // SLURM string
		}

		// --------------------------------------------------------------------------------
		// Optimize cost/gamma-step
		// --------------------------------------------------------------------------------
		summarize := NewSummarizeCostGammaPerf(wf, "summarize_cost_gamma_perf_"+geneLC, "dat/"+geneLC+"/"+geneLC+"_cost_gamma_perf_stats.tsv")
		for _, cost := range costVals {
			for _, gamma := range gammaVals {
				gene_cost_gamma := fmt.Sprintf("%s_%s_%s", geneLC, cost, gamma) // A string to make process names unique

				evalCostGamma := wf.NewProc("crossval_"+gene_cost_gamma,
					`java -jar `+cpSignPath+` crossvalidate \
									--license ../../bin/cpsign.lic \
									--cptype 1 \
									--trainfile {i:traindata} \
									--impl liblinear \
									--labels A, N \
									--nr-models {p:nrmodels} \
									--cost {p:cost} \
									--gamma {p:gamma} \
									--cv-folds {p:cvfolds} \
									--confidence {p:confidence} > {o:stats} # {p:gene}`)
				evalCostGamma.SetPathCustom("stats", func(t *sp.SciTask) string {
					c, err := strconv.ParseInt(t.Param("cost"), 10, 0)
					sp.CheckErr(err)
					g, err := strconv.ParseFloat(t.Param("gamma"), 64)
					sp.CheckErr(err)
					return t.InPath("traindata") + fmt.Sprintf(".c%03d_g%.3f", c, g) + ".stats.txt"
				})
				evalCostGamma.In("traindata").Connect(extractTargetData.Out("target_data"))
				evalCostGamma.ParamPort("nrmodels").ConnectStr("3")
				evalCostGamma.ParamPort("cvfolds").ConnectStr("10")
				evalCostGamma.ParamPort("confidence").ConnectStr("0.9")
				evalCostGamma.ParamPort("gene").ConnectStr(gene)
				evalCostGamma.ParamPort("cost").ConnectStr(cost)
				evalCostGamma.ParamPort("gamma").ConnectStr(gamma)
				if *runSlurm {
					evalCostGamma.Prepend = "salloc -A snic2017-7-89 -n 4 -t 1-00:00:00 -J evalcg_" + gene_cost_gamma + " srun " // SLURM string
				}

				summarize.In.Connect(evalCostGamma.Out("stats"))
			}
		}
		selectBest := NewBestEffCostGamma(wf, "select_best_cost_gamma_"+geneLC, '\t', false, 0)
		selectBest.InCSVFile.Connect(summarize.OutCostGammaStats)

		// --------------------------------------------------------------------------------
		// Pre-compute step
		// --------------------------------------------------------------------------------
		cpSignPrecomp := wf.NewProc("cpsign_precomp_"+geneLC,
			`java -jar `+cpSignPath+` precompute \
									--license ../../bin/cpsign.lic \
									--cptype 1 \
									--trainfile {i:traindata} \
									--labels A, N \
									--model-out {o:model} \
									--model-name "`+gene+` target profile"`)
		cpSignPrecomp.In("traindata").Connect(extractTargetData.Out("target_data"))
		cpSignPrecomp.SetPathExtend("traindata", "model", ".precomp.mdl")
		if *runSlurm {
			cpSignPrecomp.Prepend = "salloc -A snic2017-7-89 -n 4 -t 1-00:00:00 -J precmp_" + geneLC + " srun " // SLURM string
		}

		// --------------------------------------------------------------------------------
		// Train step
		// --------------------------------------------------------------------------------
		cpSignTrain := wf.NewProc("cpsign_train_"+geneLC,
			`java -jar `+cpSignPath+` train \
									--license ../../bin/cpsign.lic \
									--cptype 1 \
									--modelfile {i:model} \
									--labels A, N \
									--impl liblinear \
									--nr-models {p:acpfolds} \
									--cost {p:cost} \
									--gamma {p:gamma} \
									--model-out {o:model} \
									--model-name "{p:gene} target profile" # Efficiency: {p:efficiency}`)
		cpSignTrain.In("model").Connect(cpSignPrecomp.Out("model"))
		cpSignTrain.ParamPort("acpfolds").ConnectStr("10")
		cpSignTrain.ParamPort("cost").Connect(selectBest.OutBestCost)
		cpSignTrain.ParamPort("gamma").Connect(selectBest.OutBestGamma)
		cpSignTrain.ParamPort("gene").ConnectStr(gene)
		cpSignTrain.ParamPort("efficiency").Connect(selectBest.OutBestEfficiency)
		cpSignTrain.SetPathCustom("model", func(t *sp.SciTask) string {
			return fmt.Sprintf("dat/final_models/%s_c%s_g%s_acpfolds%s.mdl",
				str.ToLower(t.Param("gene")),
				t.Param("cost"),
				t.Param("gamma"),
				t.Param("acpfolds"))
		})
		if *runSlurm {
			cpSignTrain.Prepend = "salloc -A snic2017-7-89 -n 4 -t 1-00:00:00 -J train_" + geneLC + " srun " // SLURM string
		}

		//paramPrinter := NewParamPrinter(wf, "param_printer_"+geneLC, "dat/best_cost_gamma_"+geneLC+".txt")
		//paramPrinter.GetParamPort("cost").Connect(selectBest.OutBestCost)
		//paramPrinter.GetParamPort("gamma").Connect(selectBest.OutBestGamma)
		//paramPrinter.GetParamPort("efficiency").Connect(selectBest.OutBestEfficiency)

		wf.ConnectLast(cpSignTrain.Out("model"))
	}

	// --------------------------------
	// Run the pipeline!
	// --------------------------------
	wf.Run()
}

// --------------------------------------------------------------------------------

// SummarizeCostGammaPerf is specialized a SciPipe Process that reads output
// from cpSign status output to extract information about the efficiency and
// validity of generated models for given cost and gamma values
type SummarizeCostGammaPerf struct {
	In                *sp.FilePort
	OutCostGammaStats *sp.FilePort
	ProcName          string
	FileName          string
}

func NewSummarizeCostGammaPerf(wf *sp.Workflow, name string, filename string) *SummarizeCostGammaPerf {
	bcgs := &SummarizeCostGammaPerf{
		In:                sp.NewFilePort(),
		OutCostGammaStats: sp.NewFilePort(),
		ProcName:          name,
		FileName:          filename,
	}
	wf.AddProc(bcgs)
	return bcgs
}

func (p *SummarizeCostGammaPerf) Name() string {
	return p.ProcName
}

func (p *SummarizeCostGammaPerf) Run() {
	defer p.OutCostGammaStats.Close()
	go p.In.RunMergeInputs()

	outIp := sp.NewInformationPacket(p.FileName)

	if outIp.Exists() {
		sp.Info.Printf("Process %s: Out-target %s already exists, so not skipping\n", p.Name(), outIp.GetPath())
	} else {
		// Set up regexes
		rEffic, err := regexp.Compile("Efficiency=([0-9.]+)")
		sp.CheckErr(err)

		rValid, err := regexp.Compile("Validity=([0-9.]+)")
		sp.CheckErr(err)

		outStr := "Efficiency\tValidity\tCost\tGamma\tGene\n"
		for iip := range p.In.InChan {
			dat := string(iip.Read())

			efficiency := 0.0
			validity := 0.0

			effMatches := rEffic.FindStringSubmatch(dat)
			if len(effMatches) > 1 {
				efficiency, err = strconv.ParseFloat(effMatches[1], 64)
				sp.CheckErr(err)
			}

			validMatches := rValid.FindStringSubmatch(dat)
			if len(validMatches) > 1 {
				validity, err = strconv.ParseFloat(validMatches[1], 64)
				sp.CheckErr(err)
			}

			auditInfo := iip.GetAuditInfo()

			cost := auditInfo.Params["cost"]
			gamma := auditInfo.Params["gamma"]
			gene := auditInfo.Params["gene"]

			infoString := fmt.Sprintf("%.3f\t%.3f\t%s\t%s\t%s\n", efficiency, validity, cost, gamma, gene)
			outStr = outStr + infoString
		}
		ioutil.WriteFile(p.FileName, []byte(outStr), 0644)
	}

	p.OutCostGammaStats.Send(outIp)
}

func (p *SummarizeCostGammaPerf) IsConnected() bool {
	return p.In.IsConnected() && p.OutCostGammaStats.IsConnected()
}

// --------------------------------------------------------------------------------

type BestEffCostGamma struct {
	ProcName          string
	InCSVFile         *sp.FilePort
	OutBestCost       *sp.ParamPort
	OutBestGamma      *sp.ParamPort
	OutBestEfficiency *sp.ParamPort
	Separator         rune
	Header            bool
	ColumnIndex       int // Which column to check for max value
}

func NewBestEffCostGamma(wf *sp.Workflow, procName string, separator rune, header bool, columnIndex int) *BestEffCostGamma {
	sbcr := &BestEffCostGamma{
		ProcName:          procName,
		InCSVFile:         sp.NewFilePort(),
		OutBestCost:       sp.NewParamPort(),
		OutBestGamma:      sp.NewParamPort(),
		OutBestEfficiency: sp.NewParamPort(),
		Separator:         separator,
		Header:            header,
		ColumnIndex:       columnIndex,
	}
	wf.AddProc(sbcr)
	return sbcr
}

func (p *BestEffCostGamma) Name() string {
	return p.ProcName
}

func (p *BestEffCostGamma) Run() {
	defer p.OutBestCost.Close()
	defer p.OutBestGamma.Close()
	defer p.OutBestEfficiency.Close()
	go p.InCSVFile.RunMergeInputs()

	for iip := range p.InCSVFile.InChan {
		csvData := iip.Read()

		bytesReader := bytes.NewReader(csvData)
		csvReader := csv.NewReader(bytesReader)
		csvReader.Comma = p.Separator

		var max float64
		var maxCost int64
		var maxGamma float64

		i := 0
		for {
			rec, err := csvReader.Read()
			if err != nil {
				break
			}
			i++
			if i == 1 && !p.Header {
				continue
			}
			eff, err := strconv.ParseFloat(rec[p.ColumnIndex], 64)
			sp.CheckErr(err)
			if eff > max {
				max = eff

				maxCost, err = strconv.ParseInt(rec[2], 10, 0)
				sp.CheckErr(err)

				maxGamma, err = strconv.ParseFloat(rec[3], 64)
				sp.CheckErr(err)
			}
		}
		sp.Debug.Printf("Final max efficiency: %f (For: Cost:%03d, Gamma:%.3f)\n", max, maxCost, maxGamma)
		p.OutBestCost.Send(fmt.Sprintf("%d", maxCost))
		p.OutBestGamma.Send(fmt.Sprintf("%.3f", maxGamma))
		p.OutBestEfficiency.Send(fmt.Sprintf("%.3f", max))
	}
}

func (p *BestEffCostGamma) IsConnected() bool {
	return p.InCSVFile.IsConnected() && p.OutBestCost.IsConnected() && p.OutBestGamma.IsConnected() && p.OutBestEfficiency.IsConnected()
}

// --------------------------------------------------------------------------------

type ParamPrinter struct {
	sp.SciProcess
	ProcName           string
	InParamPorts       map[string]*sp.ParamPort
	OutBestParamsFile  *sp.FilePort
	BestParamsFileName string
}

func NewParamPrinter(wf *sp.Workflow, procName string, fileName string) *ParamPrinter {
	pp := &ParamPrinter{
		ProcName:           procName,
		InParamPorts:       make(map[string]*sp.ParamPort),
		OutBestParamsFile:  sp.NewFilePort(),
		BestParamsFileName: fileName,
	}
	wf.AddProc(pp)
	return pp
}

func (p *ParamPrinter) GetParamPort(portName string) *sp.ParamPort {
	if p.InParamPorts[portName] == nil {
		p.InParamPorts[portName] = sp.NewParamPort()
	}
	return p.InParamPorts[portName]
}

func (p *ParamPrinter) Name() string {
	return p.ProcName
}

func (p *ParamPrinter) Run() {
	defer p.OutBestParamsFile.Close()

	oip := sp.NewInformationPacket(p.BestParamsFileName)
	if !oip.Exists() && !oip.TempFileExists() {
		rows := []map[string]string{}
		for len(p.InParamPorts) > 0 {
			row := map[string]string{}
			for pname, pport := range p.InParamPorts {
				param, ok := <-pport.Chan
				if !ok {
					delete(p.InParamPorts, pname)
					continue
				}
				row[pname] = param
			}
			rows = append(rows, row)
		}

		var outContent string

		for _, row := range rows {
			for name, val := range row {
				outContent += fmt.Sprintf("%s=%s\n", name, val)
			}
		}
		oip.WriteTempFile([]byte(outContent))
		oip.Atomize()
	} else {
		sp.Info.Printf("Target file (or temp file) exists for: %s, so skipping\n", oip.GetPath())
	}

	p.OutBestParamsFile.Send(oip)
}
