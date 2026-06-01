package docs

import _ "embed"

//go:embed api/openapi.yaml
var OpenAPISpec []byte
