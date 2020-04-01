# alpine:edge required for smartmontools 7.0
# alpine:latest (mine could be older) had 6.6
FROM alpine:edge

WORKDIR /varasto

VOLUME /varasto-db

ENTRYPOINT ["sto"]

CMD ["server"]

# symlink /root/varastoclient-config.json to /varasto-db/.. because it's stateful.
# this config is used for server subsystems (thumbnailing, FUSE projector) to communicate
# with the server.

RUN mkdir -p /varasto \
	&& ln -s /varasto/sto /bin/sto \
	&& ln -s /varasto-db/varastoclient-config.json /root/varastoclient-config.json \
	&& apk add --update smartmontools fuse \
	&& echo '{"db_location": "/varasto-db/varasto.db"}' > /varasto/config.json \
	&& mkdir /mnt/stofuse

COPY rel/sto_linux-amd64 /varasto/sto

ADD rel/public.tar.gz /varasto/

RUN chmod +x /varasto/sto
