# Developing a Traefik plugin at Local Mode

插件使用的第三方库:
- [gmsm](github.com/tjfoc/gmsm)
- [redis](github.com/piaohao/godis)

Traefik also offers a developer mode that can be used for temporary testing of plugins not hosted on GitHub.

The plugins must be placed in `./plugins-local` directory,
which should be in the working directory of the process running the Traefik binary.
The source code of the plugin should be organized as follows:


```
traefik.yml
dynamic_conf.yml
./plugins-local/
    └── src
        └── github.com
            └── jack
                └── gmsmPlugin
                    ├── main.go
                    ├── go.mod
                    ├── Makefile
                    ├── .traefik.yml
                    ├── vendor
                    └── README.md
```

traefik.yml
```yaml
# Static configuration

api:
  dashboard: true
  insecure: true

log:
  level: DEBUG

entryPoints:
  web:
    address: ":80"

providers:
  file:
    filename: "dynamic_conf.yml"

experimental:
  localPlugins:
    gmsmPlugin:
      moduleName: github.com/jack/gmsmPlugin

```

(In the above example, the `gmsmPlugin` plugin will be loaded from the path `./plugins-local/src/github.com/jack/gmsmPlugin`.)

dynamic_conf.yml
```yaml
# Dynamic configuration

http:
  routers:
    my-router:
      rule: host(`localhost`)
      service: service-foo
      entryPoints:
        - web
      middlewares:
        - gmsmPlugin

  services:
    service-foo:
      loadBalancer:
        servers:
          - url: http://127.0.0.1:5000

  middlewares:
    gmsmPlugin:
      plugin:
        gmsmPlugin:
          smAlgorithm: "SM3"

```

## Defining a Plugin

A plugin package must define the following exported Go objects:

- A type `type Config struct { ... }`. The struct fields are arbitrary.
- A function `func CreateConfig() *Config`.
- A function `func New(ctx context.Context, next http.Handler, config *Config, name string) (http.Handler, error)`.

```go
// Package example a example plugin.
package example

import (
	"context"
	"net/http"
)

// Config the plugin configuration.
type Config struct {
	// ...
}

// CreateConfig creates the default plugin configuration.
func CreateConfig() *Config {
	return &Config{
		// ...
	}
}

// Example a plugin.
type Example struct {
	next     http.Handler
	name     string
	// ...
}

// New created a new plugin.
func New(ctx context.Context, next http.Handler, config *Config, name string) (http.Handler, error) {
	// ...
	return &Example{
		// ...
	}, nil
}

func (e *Example) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	// ...
	e.next.ServeHTTP(rw, req)
}
```

## 参考地址

https://github.com/traefik/plugindemo