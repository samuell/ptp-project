# Without Drugbank Experiment - TODO and Journal

- [x] Copy Go files from fillup-vs-not experiment
- [x] Update with path fixes from fillup-vs-not experiment
- [x] Clean up copied Go files of stuff not relevant to this experiment
- [x] Implement a map for looking up target/gene-specific cost values (since
  this is extracted in a previous workflow, and we don't want to re-run the
  cost-search)
  - [x] Decide if we should do this by referring to the output of the
    previous experiment, and so get the full audit trail of that file into
    outputs of this workflow?
    - Went with just hard-coded values right now
- [x] Include relevant code (for figuring out drugbank active compounds) from
  the drugbank-vs-not experiment
- [x] Add extraction and merge components from excapedb-vs-drugbank experiement
- [x] Make the merge ID file have only one column
- [x] Extract only withdrawn, and approved only as to fill up to 1000 molecules
- [x] Create filtering component
  - Some hints on how to do it:
    https://stackoverflow.com/questions/14062402/awk-using-a-file-to-filter-another-one-out-tr
- [x] Fix bug that duplicate IDs of the same molecule can occur, because
  both CHEMBL and PubChem IDs are merged too simply right now (molecules need
  to be kept together when we want to select how many to pick, etc)
- [x] Fix bug from the fact that approved/withdrawn status in drugbank raw
  data is not mutually exclusive
- [x] Fix out of memory error from SLURM, on the remove_conflicting step
- [x] Save excapeDB dataset port as a variable representing the excapedb
  dataset (new approach to easier workflow authoring I just realized)
- [x] Fix bug: We want to select drugbank molecules for removal, that
  actually are available in ExcapeDB, so that we can use it to validate both
  excapedb active/nonactive, and drugbank approved/withdrawn.
- [x] Add cost values for gene names not in the "bowes44min100percls_small" dataset, from
      exp/20180326-fillup-propertrain/res/final_models_summary_sorted.tsv
- [x] Implement component to include audit log in model files
  - But there's something weird with SciPipe. In 0.6.1, the workflow ends in
    a deadlock when files exist, and in the latest commit in develop, it seems
    tasks are not finished correctly.
  - In concrete terms, it seems the problem is with CustomExecute, in the
    latest develop commit.
- [x] Include audit log in .jar files.
- [x] Implement usage of staffan's new `--percentiles` and
      `--percentilesfile` flags, to make coloring work in the chemical graph in the
      web UI etc.
- [x] Create calibration plots
- Did some counting of entries in data files:

  ```bash
  70448221 ../../raw/pubchem.chembl.dataset4publication_inchi_smiles.gisa.tsv
  70365153 dat/excapedb.gisa_wo_drugbank.tsv
  70361830 dat/excapedb.gisa_wo_drugbank.dedup.tsv
  ```

  - Which means:
    - 83068 entries were removed because of filtering out 1000 drugbank molecules
    - 3323 entries were removed in the deduplication step (to create  the .dedup.tsv file)
- [x] Re-arrange workflow so that same 'remove conflicting' component is used
      to prepare data for both training and validation
- [x] Fix the broken removeConflicting component. Now, the conflict detection
      doesn't work since the id column was added and messes up the sorting, so that
      the same Gene / Smiles combinations don't end up close to each other.
- [>] Fix the broken extraction of removed drugbank compounds. We should
      select max one SMILES per Chembl ID / PubChem ID pair. We also need to
      extract a separate file per target.