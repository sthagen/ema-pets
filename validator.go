// Copyright (C) 2022 Emanuele Rocca
//
// Pets configuration file validator. Given a list of in-memory PetsFile(s),
// see if our sanity constraints are met. For example, we do not want multiple
// files to be installed to the same destination path. Also, all validation
// commands must succeed.

package main

import (
	"fmt"
	"io/fs"
	"strings"
)

// CheckGlobalConstraints validates assumptions that must hold across all
// configuration files.
func CheckGlobalConstraints(files []*PetsFile) error {
	// Keep the seen PetsFiles in a map so we can:
	// 1) identify and print duplicate sources
	// 2) avoid slices.Contains which is only in Go 1.18+ and not even bound to
	//    the Go 1 Compatibility Promise™
	seen := make(map[string]*PetsFile)

	for _, pf := range files {
		other, exist := seen[pf.Dest]
		if exist {
			return fmt.Errorf("ERROR: duplicate definition for '%s': '%s' and '%s'\n", pf.Dest, pf.Source, other.Source)
		}
		seen[pf.Dest] = pf
	}

	return nil
}

func pkgIsValid(pf *PetsFile) bool {
	aptCache := NewCmd([]string{"apt-cache", "policy", pf.Pkg})
	stdout, _, err := RunCmd(aptCache)

	if err != nil {
		fmt.Printf("ERROR: PkgIsValid command %s failed: %s\n", aptCache, err)
		return false
	}

	if strings.HasPrefix(stdout, pf.Pkg) {
		// Return true if the output of apt-cache policy begins with Pkg
		fmt.Printf("DEBUG: %s is a valid package name\n", pf.Pkg)
		return true
	} else {
		fmt.Printf("ERROR: %s is not an available package\n", pf.Pkg)
		return false
	}
}

// runPre returns true if the pre-update validation command passes, or if it
// was not specificed at all. The boolean argument pathErrorOK controls whether
// or not we want to fail if the validation command is not around.
func runPre(pf *PetsFile, pathErrorOK bool) bool {
	if pf.Pre == nil {
		return true
	}

	// Run 'pre' validation command, append Source filename to
	// arguments.
	// eg: /usr/sbin/sshd -t -f sample_pet/ssh/sshd_config
	pf.Pre.Args = append(pf.Pre.Args, pf.Source)

	err := pf.Pre.Run()

	_, pathError := err.(*fs.PathError)

	if err == nil {
		fmt.Printf("INFO: pre-update command %s successful\n", pf.Pre.Args)
		return true
	} else if pathError && pathErrorOK {
		// The command has failed because the validation command itself is
		// missing. This could be a chicken-and-egg problem: at this stage
		// configuration is not validated yet, hence any "package" directives
		// have not been applied.  Do not consider this as a failure, for now.
		fmt.Printf("INFO: pre-update command %s failed due to PathError. Ignoring for now\n", pf.Pre.Args)
		return true
	} else {
		fmt.Printf("ERROR: pre-update command %s: %s\n", pf.Pre.Args, err)
		return false
	}
}

// CheckLocalConstraints validates assumptions that must hold for the
// individual configuration files. An error in one file means we're gonna skip
// it but proceed with the rest. The function returns a slice of files for
// which validation passed.
func CheckLocalConstraints(files []*PetsFile, pathErrorOK bool) []*PetsFile {
	var goodPets []*PetsFile

	for _, pf := range files {
		fmt.Printf("DEBUG: validating %s\n", pf.Source)

		// Check if the specified package exists
		if !pkgIsValid(pf) {
			continue
		}

		// Check pre-update validation command
		if !runPre(pf, pathErrorOK) {
			continue
		}

		fmt.Printf("DEBUG: valid configuration file: %s\n", pf.Source)
		goodPets = append(goodPets, pf)
	}

	return goodPets
}
