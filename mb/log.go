package mb

import (
	"fmt"

	"github.com/fatih/color"
)

const prefixWidth = 10

var boldMsg = color.New(color.Bold)

func logMsg(prefix, message string) {
	if len(prefix) > prefixWidth {
		prefix = prefix[:prefixWidth]
	}
	alignedPrefix := fmt.Sprintf("%*s", prefixWidth, prefix)

	greenBold := color.New(color.FgGreen, color.Bold)
	fmt.Printf("%s %s\n", greenBold.Sprint(alignedPrefix), boldMsg.Sprint(message))
}

func logMsgf(prefix, format string, v ...any) {
	logMsg(prefix, fmt.Sprintf(format, v...))
}

func logErr(prefix, message string) {
	if len(prefix) > prefixWidth {
		prefix = prefix[:prefixWidth]
	}
	alignedPrefix := fmt.Sprintf("%*s", prefixWidth, prefix)

	redBold := color.New(color.FgRed, color.Bold)
	fmt.Printf("%s %s\n", redBold.Sprint(alignedPrefix), boldMsg.Sprint(message))
}

func logErrf(prefix, format string, v ...any) {
	logErr(prefix, fmt.Sprintf(format, v...))
}
