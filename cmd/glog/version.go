package main

import (
	"fmt"

	"github.com/alecthomas/kong"
)

// versionFlag preserves the version output exposed before the Kong migration.
type versionFlag bool

func (versionFlag) BeforeReset(app *kong.Kong, vars kong.Vars) error {
	if _, err := fmt.Fprintf(app.Stdout, "%s version %s\n", app.Model.Name, vars["version"]); err != nil {
		return fmt.Errorf("write version: %w", err)
	}
	app.Exit(0)
	return nil
}
