build:
	go build -o bin/files/allmark ./cli
	cp -p bin/files/allmark documentation/documents

install:
	go build -o bin/files/allmark ./cli

test:
	go test ./cli ./common/... ./dataaccess/... ./model/... ./services/... ./web/...

crosscompileWin7:
	GOTOOLCHAIN=go1.19 GOOS=windows GOARCH=amd64 go build -o bin/files/win7/server.exe ./cli

crosscompileWin:
	GOTOOLCHAIN=go1.22 GOOS=windows GOARCH=amd64 go build -o bin/files/server.exe ./cli
