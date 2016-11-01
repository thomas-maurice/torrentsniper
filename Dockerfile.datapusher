FROM scratch
MAINTAINER Thomas Maurice <thomas@maurice.fr>

COPY bin/datapusher /datapusher
COPY GeoLite2-City.mmdb /GeoLite2-City.mmdb

ENTRYPOINT ["/datapusher"]
