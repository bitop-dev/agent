FROM alpine:3.21

ARG TARGETARCH

RUN apk add --no-cache ca-certificates tzdata

COPY dist/${TARGETARCH}/agent-linux-${TARGETARCH} /usr/local/bin/agent

RUN chmod +x /usr/local/bin/agent

# Default config directory
RUN mkdir -p /root/.agent

EXPOSE 9898

ENTRYPOINT ["agent"]
CMD ["serve", "--addr", ":9898"]
