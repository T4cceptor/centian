FROM t4ce/centian:latest

WORKDIR /workspace

COPY demo/processors/requirements.txt /tmp/demo-requirements.txt
RUN python -m pip install --no-cache-dir -r /tmp/demo-requirements.txt \
  && rm -f /tmp/demo-requirements.txt
COPY demo/processors/src/ /opt/centian/processors/

RUN chmod +x /usr/local/bin/centian

ENTRYPOINT ["centian", "start"]
