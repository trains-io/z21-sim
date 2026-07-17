module github.com/trains-io/z21-sim-go

go 1.24

require (
	github.com/trains-io/z21.go v0.0.0
	google.golang.org/grpc v1.68.1
	google.golang.org/protobuf v1.36.5
)

require (
	golang.org/x/net v0.38.0 // indirect
	golang.org/x/sys v0.32.0 // indirect
	golang.org/x/text v0.23.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20241209162323-e6fa225c2576 // indirect
)

replace github.com/trains-io/z21.go => ../z21.go
