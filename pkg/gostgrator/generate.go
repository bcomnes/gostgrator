//go:generate sh -c "printf 'package gostgrator\\n\\nvar (\\n\\tVersion   = \\\"%s\\\"\\n\\tGitCommit = \\\"%s\\\"\\n)\\n' \"$(git describe --tags --always --dirty)\" \"$(git rev-parse HEAD)\" > version.go"
package gostgrator
