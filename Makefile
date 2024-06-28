.PHONY: vendor clean

export GO111MODULE=on

vendor:
	go mod vendor

clean:
	rm -rf ./vendor