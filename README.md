 TorrentSniper
==============

A troll project where we recoded the french [HADOPI](https://hadopi.fr)

This project's aim is to collect data about torrent that are being downloaded, for
statistical purposes.

How this project works exactly is detailed
[there](https://thomas.maurice.fr/blog/2016/10/31/how-we-recoded-hadopi/).

 Compiling it
-------------
Just type `make`, it will just work and create the binaries in the `bin/` directory.

 Configuring it
---------------
You need to create basically 3 configuration files, either in `/etc` or in the
current directory. They are

* `peersearcher.yml`
* `datapusher.yml`
* `torrentfetcher.yml`

They can have exactly the same content, except for the `PromAddr` which must be
changed if you execute all the binaries on the same host. The file is as follow:

```yaml
# Client ID used to connect to Kafka
ClientID: XXXXXXX
# Topic in which the ip addresses are pushed for processing
TorrentTopic: exemple.torrent
# Torrent where the torent magnet links are pushed
TorrentLinksTopic: exemple.torrent_links
# ElasticSearch index to use
ESIndex: torrent
ESAddress: http://elastic:9200
# Kafka host to connect to
KafkaHost: "your-kafka-host:9092"
# Prometheus monitoring endpoint address
PromAddr: ":8080"
# Redis configuration
RedisAddress: localhost:6379
RedisPassword: ""
RedisDB: 0
RedisConnectTimeout: 1s
RedisCacheTTL: 3600s
UseRedis: true
```

Note that the configuration variables can also be
passed as environment variables. For that just
caps lock them. Exemple: RedisAddress -> REDISADDRESS.
And it will just work out of the box when you
launch the binaries.

 Contributing
-------------
Feel free to make pull requests !

 Contributors
-------------
The original idea for this concept comes from [KDevroede](https://twitter.com/KDevroede).

The initial code was written by [Thomas Maurice](https://twitter.com/thomas_maurice) and instrumented
by [Timothée Germain](https://twitter.com/DarkNihilius1)
