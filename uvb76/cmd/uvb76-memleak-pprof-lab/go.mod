module github.com/s1onique/KGB/uvb76/cmd/uvb76-memleak-pprof-lab

go 1.25.0

require (
	github.com/google/pprof v0.0.0-20260709232956-b9395ee17fa0
	github.com/s1onique/KGB/uvb76 v0.1.0
	golang.org/x/tools v0.48.0
)

require (
	github.com/gorilla/mux v1.8.1 // indirect
	golang.org/x/mod v0.38.0 // indirect
	golang.org/x/net v0.57.0 // indirect
	golang.org/x/sync v0.22.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
)

replace github.com/s1onique/KGB/uvb76 => ../..
