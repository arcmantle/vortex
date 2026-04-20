module arcmantle/vortex

go 1.25.5

require (
	arcmantle/windowfocus v0.0.0
	arcmantle/windowicon v0.0.0
	github.com/webview/webview_go v0.0.0-20240831120633-6173450d4dd6
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/arcmantle/rembed v1.0.0
	github.com/creack/pty v1.1.24
	github.com/spf13/cobra v1.9.1
	golang.org/x/sys v0.42.0
)

require (
	github.com/evanw/esbuild v0.28.0 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/spf13/pflag v1.0.6 // indirect
)

replace arcmantle/windowfocus => ./windowfocus

replace arcmantle/windowicon => ./windowicon
