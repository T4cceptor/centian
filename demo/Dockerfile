FROM python:3.12-slim

WORKDIR /workspace

RUN apt-get update \
  && apt-get install -y --no-install-recommends ca-certificates nodejs npm \
  && rm -rf /var/lib/apt/lists/*

COPY build/centian /usr/local/bin/centian
COPY demo/requirements.txt /tmp/demo-requirements.txt
RUN python -m pip install --no-cache-dir -r /tmp/demo-requirements.txt \
  && rm -f /tmp/demo-requirements.txt
RUN chmod +x /usr/local/bin/centian

ENTRYPOINT ["centian", "start"]
