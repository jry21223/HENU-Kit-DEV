package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	databaseURL := flag.String("database-url", "", "PostgreSQL connection URL override (prefer DATABASE_URL to avoid process-list exposure)")
	bankKey := flag.String("bank-key", "", "stable lowercase bank key")
	filePath := flag.String("file", "", "JSON bank file to import explicitly")
	flag.Parse()
	_ = databaseURL
	_ = bankKey
	_ = filePath
	fail(fmt.Errorf("direct activation is disabled; import through the authenticated QuizCraft Workshop, review the full version, then publish it"))
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
