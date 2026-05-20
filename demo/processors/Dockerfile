# 1 build binary
FROM golang:1.25-alpine AS builder
WORKDIR /workspace
COPY go.mod go.sum ./
COPY demo/it_ops/prompt_injection_guard ./demo/it_ops/prompt_injection_guard
RUN go build -o /opt/centian/processors/prompt-injection-guard ./demo/it_ops/prompt_injection_guard


FROM t4ce/centian:latest

WORKDIR /workspace

COPY --from=builder /opt/centian/processors/prompt-injection-guard /opt/centian/processors/prompt-injection-guard
COPY demo/processors/requirements.txt /tmp/demo-requirements.txt
RUN python -m pip install --no-cache-dir -r /tmp/demo-requirements.txt \
  && rm -f /tmp/demo-requirements.txt
COPY demo/processors/src/ /opt/centian/processors/

RUN chmod +x /usr/local/bin/centian

ENTRYPOINT ["centian", "start"]
