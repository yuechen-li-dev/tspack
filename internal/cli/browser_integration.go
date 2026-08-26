package cli

import browserintegration "github.com/yuechen-li-dev/tspack/internal/integrations/browser"

type tsclNpmContract = browserintegration.NpmContract
type tsclNpmExport = browserintegration.NpmExport
type tsclNpmComponent = browserintegration.NpmComponent
type tsclNpmMember = browserintegration.NpmMember
type tsclNpmProperty = browserintegration.NpmProperty

type browserMaterialization = browserintegration.Materialization
type browserImport = browserintegration.Import

var browserRuntimeSource = browserintegration.RuntimeSource()

func packageRootName(specifier string) string {
	return browserintegration.PackageRootName(specifier)
}

func packageSubpath(specifier string) string {
	return browserintegration.PackageSubpath(specifier)
}

func selectBrowserPackageEntry(directory string, subpath string) (string, error) {
	return browserintegration.SelectPackageEntry(directory, subpath)
}

func isCommonJSModule(directory string, entry string) bool {
	return browserintegration.IsCommonJSModule(directory, entry)
}

func browserTransformerIdentity() string {
	return browserintegration.TransformerIdentity()
}

func materializeBrowserGraph(outputDirectory string, contracts []tsclNpmContract) (browserMaterialization, error) {
	return browserintegration.MaterializeGraph(outputDirectory, contracts)
}

func writeBrowserHost(packageRoot string, outputDirectory string, entryOutputPath string, materialization browserMaterialization) error {
	return browserintegration.WriteHost(packageRoot, outputDirectory, entryOutputPath, materialization)
}
