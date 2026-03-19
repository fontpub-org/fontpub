package cli

import (
	"sort"
	"time"

	"github.com/fontpub-org/fontpub/go/internal/protocol"
)

func (a *App) now() time.Time {
	if a.Now == nil {
		return time.Now().UTC()
	}
	return a.Now()
}

func (a *App) lockfileStore() LockfileStore {
	return LockfileStore{Path: a.Config.LockfilePath()}
}

func (a *App) loadOrInitLockfile() (protocol.Lockfile, error) {
	lock, ok, err := a.lockfileStore().Load()
	if err != nil {
		return protocol.Lockfile{}, err
	}
	if ok {
		return lock, nil
	}
	return protocol.Lockfile{
		SchemaVersion: "1",
		GeneratedAt:   a.now().Format(time.RFC3339),
		Packages:      map[string]protocol.LockedPackage{},
	}, nil
}

func packageResultsToDetails(results []PackageCheckResult) map[string]any {
	items := make([]any, 0, len(results))
	for _, result := range results {
		findings := make([]any, 0, len(result.Findings))
		for _, finding := range result.Findings {
			findings = append(findings, map[string]any{
				"code":     finding.Code,
				"severity": finding.Severity,
				"subject":  finding.Subject,
				"message":  finding.Message,
				"details":  finding.Details,
			})
		}
		items = append(items, map[string]any{
			"package_id": result.PackageID,
			"ok":         result.OK,
			"findings":   findings,
		})
	}
	return map[string]any{"packages": items}
}

func sortFindings(findings []Finding) {
	sort.Slice(findings, func(i, j int) bool {
		if findings[i].Code == findings[j].Code {
			return findings[i].Message < findings[j].Message
		}
		return findings[i].Code < findings[j].Code
	})
}
