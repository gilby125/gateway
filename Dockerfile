FROM alpine:3.19

ENV TZ=Etc/UTC ROTATION_TZ=Etc/UTC

ENV DEBIAN_FRONTEND=noninteractive

RUN echo $TZ > /etc/timezone && \
    ln -snf /usr/share/zoneinfo/$TZ /etc/localtime

# Create a non-root user and group
RUN addgroup --system cligroup && adduser --system --ingroup cligroup cliuser

# Create necessary directories with proper permissions
RUN mkdir -p /var/log/gateway /etc/gateway && \
    chown -R cliuser:cligroup /var/log/gateway /etc/gateway && \
    chmod 755 /var/log/gateway /etc/gateway

ARG BINARY=gateway
COPY ${BINARY} /usr/local/bin/gw

RUN chmod +x /usr/local/bin/gw

# Set ownership of the binary to the non-root user
RUN chown cliuser:cligroup /usr/local/bin/gw

# Switch to the non-root user
USER cliuser

ENTRYPOINT ["/usr/local/bin/gw"]
