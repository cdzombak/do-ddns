# DigitalOcean-based Dynamic DNS, with client/server architecture

## Why?

There are plenty of scripts and small programs, like [anaganisk/digitalocean-dynamic-dns-ip](https://github.com/anaganisk/digitalocean-dynamic-dns-ip), which run entirely on the client. The problem there is that the client has to have a DigitalOcean API key with `write` scope, which allows anyone with access to that client to make changes throughout your DigitalOcean account.

This project's architecture is more complex, but ensures that even a shared or compromised client can only change its own dynamic DNS record and can't interfere with records for other clients or anything else in your DigitalOcean account.  

## Deployment

### DNS

You'll need two different domains, such as `a.ddns.example.net` and `aaaa.ddns.example.net`, set up to point to the same server. One (`a.ddns.example.net`) should have only an A record pointing to the server's IPv4 address; and the other (`aaaa.ddns.example.net`) should have only an AAAA record pointing to the server's IPv6 address.

### Server & Client (systemd)

Create a user and group for the service to use, and create a directory in `/etc` for configuration:

```shell script
# as root...

useradd -r -s /sbin/nologin do-ddns
mkdir /etc/do-ddns
chown root:do-ddns /etc/do-ddns
```

### Server (systemd)

```shell script
# as root...

# TODO: get latest do-ddns-server release for this arch; extract server to /usr/local/bin
chown do-ddns:do-ddns /usr/local/bin/do-ddns-server
chmod +x /usr/local/bin/do-ddns-server

cp server/deployment/.env.sample /etc/do-ddns/.env
nano /etc/do-ddns/.env # customize as needed
chown root:do-ddns /etc/do-ddns/.env

cp server/deployment/domains.json.sample /etc/do-ddns/domains.json
nano /etc/do-ddns/domains.json # customize as needed
chown root:do-ddns /etc/do-ddns/domains.json

# Install and start the server systemd unit:
cp server/deployment/do-ddns-server.service /etc/systemd/system
systemctl daemon-reload
systemctl enable do-ddns-server

# Test that the service is working:
curl -i localhost:7001/ping

# Configure nginx, or whatever frontend reverse proxy you're using:
cp server/deployment/nginx-ddns.conf /etc/nginx/sites-available/ddns.example.net
nano /etc/nginx/sites-available/ddns.example.net # customize as needed
ln -s /etc/nginx/sites-available/ddns.example.net /etc/nginx/sites-enabled/ddns.example.net

# Setup TLS as needed/desired. Depending on your nginx configuration, the following command may differ, though it'll work with the provided example nginx config file:
certbot certonly --webroot --agree-tos --no-eff-email --email me@example.com -w /var/www/letsencrypt -d a.ddns.example.net -d aaaa.ddns.example.net

systemctl reload nginx

# Test that the frontend is serving:
curl -i https://a.ddns.example.net/ping
```

### Client (systemd)

```shell script
# as root...

# TODO: get latest do-ddns-client release for this arch; extract client to /usr/local/bin
chown do-ddns:do-dnds /usr/local/bin/do-ddns-client
chmod +x /usr/local/bin/do-ddns-client

cp client/deployment/.env.sample /etc/do-ddns/.env
nano /etc/do-ddns/.env # customize as needed
chown root:do-ddns /etc/do-ddns/.env

# Install and start the client systemd unit:
cp client/deployment/do-ddns-client.service /etc/systemd/system
systemctl daemon-reload
systemctl enable do-ddns-client
```

### Client (macOS)

```shell script
# TODO: get latest do-ddns-client release for this arch; extract client to /usr/local/bin
chmod +x /usr/local/bin/do-ddns-client

cp client/deployment/org.dzombak.do-ddns-client.plist ~/Library/LaunchAgents
sudo launchctl load -w ~/Library/LaunchAgents/org.dzombak.do-ddns-client.plist
```

## Author

Chris Dzombak, [dzombak.com](https://www.dzombak.com)

- [github.com/cdzombak](https://github.com/cdzombak)
- [twitter.com/cdzombak](https://twitter.com/cdzombak)

## Credits/Thanks

I wrote this partly as an exercise in Golang, and some bits of the code are based on other open-source projects and blog posts:

- DigitalOcean API code is adapted from [anaganisk/digitalocean-dynamic-dns-ip](https://github.com/anaganisk/digitalocean-dynamic-dns-ip)
- The HTTP handler/error handling approach is adapted from [http.Handler and Error Handling in Go by Matt Silverlock](https://blog.questionable.services/article/http-handler-error-handling-revisited/)

## License

Mozilla Public License, v. 2.0. See `LICENSE` in the repository.
