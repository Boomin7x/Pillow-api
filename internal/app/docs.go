package app

import (
	"github.com/gofiber/fiber/v2"
	"github.com/kodiahbertrand/pillow/docs"
)

const swaggerUI = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>Pillow API</title>
  <link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css" />
</head>
<body>
<div id="swagger-ui"></div>
<script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
<script>
  window.onload = () => {
    SwaggerUIBundle({
      url: "/docs/openapi.yaml",
      dom_id: "#swagger-ui",
      presets: [SwaggerUIBundle.presets.apis, SwaggerUIBundle.SwaggerUIStandalonePreset],
      layout: "BaseLayout",
      deepLinking: true,
      docExpansion: "list",
      persistAuthorization: true,
    });
  };
</script>
</body>
</html>`

func registerDocsRoutes(f *fiber.App) {
	f.Get("/docs/openapi.yaml", func(c *fiber.Ctx) error {
		c.Set(fiber.HeaderContentType, "application/yaml; charset=utf-8")
		return c.Send(docs.OpenAPISpec)
	})

	serveUI := func(c *fiber.Ctx) error {
		c.Set(fiber.HeaderContentType, fiber.MIMETextHTMLCharsetUTF8)
		return c.SendString(swaggerUI)
	}

	f.Get("/docs", func(c *fiber.Ctx) error {
		return c.Redirect("/docs/", fiber.StatusMovedPermanently)
	})
	f.Get("/docs/", serveUI)
	f.Get("/docs/index.html", serveUI)
}
