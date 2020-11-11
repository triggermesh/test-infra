##
## Package ramp-requests.py together with vegeta, tools/cegen and tini.
#

FROM golang:1.15 as cegen

WORKDIR /cegen

COPY perf/tools/cegen/ ./
RUN go build -v ./

# --------------------------------

FROM golang:1.15 as vegeta

ENV VEGETA_VERSION 12.8.4

WORKDIR /vegeta

RUN curl -LO https://github.com/tsenart/vegeta/releases/download/v${VEGETA_VERSION}/vegeta_${VEGETA_VERSION}_linux_amd64.tar.gz && \
    curl -LO https://github.com/tsenart/vegeta/releases/download/v${VEGETA_VERSION}/vegeta_${VEGETA_VERSION}_checksums.txt && \
    grep vegeta_${VEGETA_VERSION}_linux_amd64.tar.gz vegeta_${VEGETA_VERSION}_checksums.txt | shasum -c && \
    tar -xzvf vegeta_${VEGETA_VERSION}_linux_amd64.tar.gz vegeta && \
    chown -v root:root vegeta

# --------------------------------

FROM golang:1.15 as tini

ENV TINI_VERSION 0.19.0

WORKDIR /tini

RUN curl -LO https://github.com/krallin/tini/releases/download/v${TINI_VERSION}/tini-amd64 && \
    curl -LO https://github.com/krallin/tini/releases/download/v${TINI_VERSION}/tini-amd64.sha1sum && \
    shasum -c tini-amd64.sha1sum && \
    mv tini-amd64 tini && \
    chmod -v +x tini

# --------------------------------

FROM gcr.io/distroless/python3-debian10:nonroot

COPY perf/load-ramping/container/entrypoint.sh /usr/local/bin/
COPY perf/load-ramping/container/cat.py /usr/local/bin/
COPY perf/load-ramping/ramp-requests.py /usr/local/bin/
COPY perf/load-ramping/sample-ce-data.json /

COPY --from=cegen /cegen/cegen /usr/local/bin/
COPY --from=vegeta /vegeta/vegeta /usr/local/bin/
COPY --from=tini /tini/tini /usr/local/bin/

ENV TARGET_URL http://localhost
ENV CE_TYPE io.triggermesh.perf.drill
ENV CE_SOURCE cegen
ENV CE_DATA @/sample-ce-data.json

# The ramp-requests.py script runs within a /bin/sh shell so it can read input from a pipe. By default, only the "sh"
# process would receive a SIGTERM upon container termination, and this signal wouldn't be propagated to
# ramp-requests.py. With the -g flag, every process in the child process group receives the signal.
ENTRYPOINT ["tini", "-g", "--", "entrypoint.sh"]
