FROM goacme/lego:latest

# Install cron and socat for HAProxy communication
RUN apk add --no-cache cron socat

# Copy only the renewal script and cron file
COPY renew.sh /usr/local/bin/renew.sh
RUN chmod +x /usr/local/bin/renew.sh

COPY lego.cron /etc/cron.d/lego
RUN chmod 0644 /etc/cron.d/lego && crontab /etc/cron.d/lego

CMD ["crond", "-f", "-d", "8"]
