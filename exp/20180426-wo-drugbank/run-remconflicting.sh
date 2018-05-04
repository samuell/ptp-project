#!/bin/bash -l
#SBATCH -A snic2017-7-89
#SBATCH -p core
#SBATCH -n 4
#SBATCH -J rem_conflicting
#SBATCH -t 4:00:00
#SBATCH --mail-user samuel.lampa@farmbio.uu.se
#SBATCH --mail-type BEGIN,FAIL,END
module load java/sun_jdk1.8.0_92
module load R/3.4.0
go run wo_drugbank_wf.go components.go -threads 1 -maxtasks 2 -procs "remove_conflicting" 2>&1 | tee log/scipipe-$(date +%Y%m%d-%H%M%S).log # -debug
