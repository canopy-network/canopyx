package controller

import (
	"net/http"
	"os"
)

// HandleSwaggerUI serves the Swagger UI HTML page at the root path
func (c *Controller) HandleSwaggerUI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	html := `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>CanopyX API Documentation</title>
    <link rel="stylesheet" type="text/css" href="https://unpkg.com/swagger-ui-dist@5.11.0/swagger-ui.css">
    <style>
        body {
            margin: 0;
            padding: 0;
        }
        .topbar {
            display: none;
        }
    </style>
</head>
<body>
    <div id="swagger-ui"></div>
    <script src="https://unpkg.com/swagger-ui-dist@5.11.0/swagger-ui-bundle.js"></script>
    <script src="https://unpkg.com/swagger-ui-dist@5.11.0/swagger-ui-standalone-preset.js"></script>
    <script>
        window.onload = function() {
            window.ui = SwaggerUIBundle({
                url: "/openapi.yaml",
                dom_id: '#swagger-ui',
                deepLinking: true,
                presets: [
                    SwaggerUIBundle.presets.apis,
                    SwaggerUIStandalonePreset
                ],
                plugins: [
                    SwaggerUIBundle.plugins.DownloadUrl
                ],
                layout: "StandaloneLayout"
            });
        };
    </script>
</body>
</html>`

	w.Write([]byte(html))
}

// HandleOpenAPISpec serves the OpenAPI YAML specification file
func (c *Controller) HandleOpenAPISpec(w http.ResponseWriter, r *http.Request) {
	content, err := os.ReadFile("./docs/openapi.yaml")
	if err != nil {
		http.Error(w, "OpenAPI spec not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/yaml")
	w.Write(content)
}