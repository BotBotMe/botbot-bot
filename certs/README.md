The certs included in this repo has been with `generate_cert` from the GO
standard library.

```
generate_cert -ca=true -duration=8760h0m0s -host="127.0.0.1" -start-date="Jan 1 15:04:05 2014"
```

The generate_cert can be build like this

```
go build $GOROOT/src/pkg/crypto/tls/generate_cert.go
```
