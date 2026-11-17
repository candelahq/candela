#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/../../.."

go test ./pkg/costcalc/ -run TestCacheCostReconcilesWithInputCost -count=1
go test ./pkg/costcalc/ -run '^$' -fuzz FuzzCacheCostReconciles -fuzztime=20s
