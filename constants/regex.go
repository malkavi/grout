package constants

import "regexp"

var TagRegex = regexp.MustCompile(`\((.*?)\)`)
var OrderedFolderRegex = regexp.MustCompile(`\d+\)\s`)
