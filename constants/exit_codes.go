package constants

import (
	gaba "github.com/BrandonKowalski/gabagool/v2/pkg/gabagool"
)

const (
	ExitCodeEditMappings             gaba.ExitCode = 100
	ExitCodeSaveSync                 gaba.ExitCode = 101
	ExitCodeSearch                   gaba.ExitCode = 200
	ExitCodeClearSearch              gaba.ExitCode = 201
	ExitCodeCollections              gaba.ExitCode = 300
	ExitCodeBackToCollection         gaba.ExitCode = 301
	ExitCodeBackToCollectionPlatform gaba.ExitCode = 302
	ExitCodeNoResults                gaba.ExitCode = 404
)
