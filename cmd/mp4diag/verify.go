package main

import (
	"flag"
	"fmt"

	"mp4diag"
)

func runVerifyFixtures(args []string) error {
	fs := flag.NewFlagSet("verify-fixtures", flag.ContinueOnError)
	manifest := fs.String("manifest", "injections/manifest.json", "注入预期清单")
	if err := fs.Parse(args); err != nil {
		return err
	}
	return verifyFixtureManifest(*manifest)
}

func verifyFixtureManifest(path string) error {
	if err := mp4diag.VerifyManifest(path); err != nil {
		return err
	}
	fmt.Printf("all fixtures match %s\n", path)
	return nil
}
