package packages

import (
	_ "embed"
	"path"
	"strings"

	"github.com/RimuruChan/Vertex/server/internal/modules/authoring/domain"
)

// This fragment is also embedded in the isolated worker renderer. Both paths
// are exercised by the native/external PDF interoperability tests.
//
//go:embed adapters/polygon.tex
var polygonLayout string

type polygonStatementLocation struct{ sourceDirectory, targetDirectory string }

func polygonLocations(tree domain.ContentTree, target string) map[string]polygonStatementLocation {
	result := map[string]polygonStatementLocation{}
	for _, entry := range tree.Entries {
		if entry.Kind == domain.EntryStatement && entry.Attributes["format"] == "tex" && entry.Attributes["dialect"] == "polygon" {
			result[entry.ID] = polygonStatementLocation{path.Dir(entry.Path), target + "/polygon-" + entry.Attributes["language"]}
		}
	}
	return result
}

func (location polygonStatementLocation) resource(name string) string {
	if location.sourceDirectory == "." {
		return location.targetDirectory + "/" + name
	}
	if !strings.HasPrefix(name, location.sourceDirectory+"/") {
		return ""
	}
	return location.targetDirectory + "/" + strings.TrimPrefix(name, location.sourceDirectory+"/")
}

func polygonWrapper(entry domain.TreeEntry, location polygonStatementLocation) (string, error) {
	// These names are inserted into TeX markup; don't let filename catcodes alter
	// the wrapper. Ordinary package paths remain untouched in the native archive.
	name := path.Base(entry.Path)
	if !standardComponent.MatchString(name) {
		return "", invalid("Polygon 题面文件名需要先规范化：%s", entry.Path)
	}
	directory := path.Base(location.targetDirectory)
	return "\n\\begingroup\n" + polygonLayout + "\n\\ifdefined\\subimport\n\\subimport{" + directory + "/}{" + name + "}\n\\else\n\\makeatletter\\def\\input@path{{" + directory + "/}}\\makeatother\n\\graphicspath{{" + directory + "/}}\n\\input{" + directory + "/" + name + "}\n\\fi\n\\endgroup\n", nil
}
