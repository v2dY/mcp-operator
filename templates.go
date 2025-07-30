package kmcp
import _ "embed"

//go:embed templates/buildah-script.sh
var BuildahScriptTemplate string

//go:embed templates/package.json
var PackageJsonTemplate string

//go:embed templates/entrypoint.sh
var EntrypointTemplate string