all: torrentfetcher datapusher peersearcher

deps:
	/bin/sh -c "cd datapusher && go get -d"
	/bin/sh -c "cd torrentfetcher && go get -d"
	/bin/sh -c "cd peersearcher && go get -d"

bindir:
	if ! [ -d ./bin ]; then mkdir ./bin; fi;

datapusher: deps format bindir
	/bin/sh -c "cd datapusher && CGO_ENABLED=0 go build -v -o ../bin/datapusher"
peersearcher: deps format bindir
	/bin/sh -c "cd peersearcher && go build -v -o ../bin/peersearcher"
torrentfetcher: deps format bindir
	/bin/sh -c "cd torrentfetcher && CGO_ENABLED=0 go build -v -o ../bin/torrentfetcher"

format:
	for directory in torrentfetcher peersearcher datapusher; do /bin/sh -c "cd $$directory && go fmt"; done;
