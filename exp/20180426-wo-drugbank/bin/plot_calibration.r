#!/usr/bin/Rscript

# ------------------------------------------------------------------------
# Commandline parsing
# ------------------------------------------------------------------------
library(getopt)
optspec = matrix(c(
  'infile', 'i', 1, 'character',
  'outfile', 'o', 1, 'character',
  'format', 'f', 1, 'character'
), byrow=TRUE, ncol=4);
opt = getopt(optspec);

# if help was asked for print a friendly message
# and exit with a non-zero error code
if ( is.null(opt$format) || is.null(opt$infile) || is.null(opt$outfile) ) {
  cat('Usage: Rscript plot_heatmap.r -i infile -o outfile -f (png|pdf)\n');
  q(status=1);
}

# ------------------------------------------------------------------------
# Create and plot the heatmap
# ------------------------------------------------------------------------
# Set output format
if (opt$format == 'png') {
	png(opt$outfile, width=1200, height=640, units="px")
} else if (opt$format =='pdf') {
	pdf(opt$outfile);
}

d <- read.csv(opt$infile, sep = '\t', header = TRUE);

plot(d$confidence, d$accuracy)
mtext("Accuracy versus Confidence")

dev.off()
# Avoid sending non-zero exit values on exit
quit(save = "no", status = 0, runLast = FALSE)