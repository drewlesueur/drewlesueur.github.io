#!/bin/bash

go list -f '{{.Deps}}'  | tr "[" " " | tr "]" " " | xargs go list -f '{{if not .Standard}}{{.ImportPath}}{{end}}' 