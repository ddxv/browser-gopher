package extractors

import (
	"log"
	"os"

	"github.com/iansinnott/browser-gopher/pkg/types"
	"github.com/iansinnott/browser-gopher/pkg/util"
)

type pathSpec struct {
	name            string
	path            string
	findDBs         func(string) ([]string, error)
	createExtractor func(name string, dbPath string) types.Extractor
}

// Build a list of relevant extractors for this system
// @todo If we want to go multi platform this is currently the place to specify
// the logic to determine paths on a per-platform basis. The extractors should
// all Just Work if they are pointed to an appropriate sqlite db.
func BuildExtractorList() ([]types.Extractor, error) {
	result := []types.Extractor{}

	pathsToTry := []pathSpec{
		{
			name:    "chrome",
			path:    util.Expanduser("~/Library/Application Support/Google/Chrome/"),
			findDBs: FindChromiumDBs,
			createExtractor: func(name, dbPath string) types.Extractor {
				return &ChromiumExtractor{Name: name, HistoryDBPath: dbPath}
			},
		},
		{
			name:    "vivaldi",
			path:    util.Expanduser("~/Library/Application Support/Vivaldi"),
			findDBs: FindChromiumDBs,
			createExtractor: func(name, dbPath string) types.Extractor {
				return &ChromiumExtractor{Name: name, HistoryDBPath: dbPath}
			},
		},
		{
			name:    "firefox",
			path:    util.Expanduser("~/Library/Application Support/Firefox/Profiles/"),
			findDBs: FindFirefoxDBs,
			createExtractor: func(name, dbPath string) types.Extractor {
				return &FirefoxExtractor{Name: name, HistoryDBPath: dbPath}
			},
		},
		{
			name: "safari",
			path: util.Expanduser("~/Library/Safari/"),
			findDBs: func(s string) ([]string, error) {
				dbPath := s + "History.db"
				if _, err := os.Stat(dbPath); err != nil {
					return nil, err
				}
				return []string{dbPath}, nil
			},
			createExtractor: func(name, dbPath string) types.Extractor {
				return &SafariExtractor{Name: name, HistoryDBPath: dbPath}
			},
		},
	}

	for _, x := range pathsToTry {
		stat, err := os.Stat(x.path)
		if err != nil || !stat.IsDir() {
			log.Println("Skipping invalid path:", x.path)
			continue
		}

		dbs, err := x.findDBs(x.path)
		if err != nil {
			return nil, err
		}
		for _, dbPath := range dbs {
			result = append(result, x.createExtractor(x.name, dbPath))
		}
	}

	return result, nil
}
