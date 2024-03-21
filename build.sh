cd cmd
GOOS=linux GOARCH=amd64 go build -o ../output/mmin-linux-x86
GOOS=linux GOARCH=arm64 go build -o ../output/mmin-linux-arm64
GOOS=windows GOARCH=amd64 go build -o ../output/mmin-win-x86
GOOS=darwin GOARCH=amd64 go build -o ../output/mmin-mac-x86
GOOS=darwin GOARCH=arm64 go build -o ../output/mmin-mac-arm64
