#!/bin/bash

curl http://localhost:9200/torrent -XDELETE
curl http://localhost:9200/torrent -XPUT
curl http://localhost:9200/torrent/address/_mapping -XPUT -d@mapping.json
