// Copyright 2025 JC-Lab
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-FEL

//go:build tools
// +build tools

package tools

import (
	_ "github.com/google/addlicense"
)

//go:generate addlicense -c "JC-Lab" -l "AGPL-3.0-or-later OR LicenseRef-FEL" -s .
