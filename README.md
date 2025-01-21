> [!IMPORTANT]  
> This project is no longer maintained.

# DigitalOcean-based Dynamic DNS, with client/server architecture

## Why?

There are plenty of scripts and small programs, like [anaganisk/digitalocean-dynamic-dns-ip](https://github.com/anaganisk/digitalocean-dynamic-dns-ip), which run entirely on the client. The problem there is that the client has to have a DigitalOcean API key with `write` scope, which allows anyone with access to that client to make changes throughout your DigitalOcean account.

This project's architecture is more complex, but ensures that even a shared or compromised client can only change its own dynamic DNS record and can't interfere with records for other clients or anything else in your DigitalOcean account.

## Features

- client/server architecture protects your DigitalOcean API key from untrustworthy clients
- supports DynDns-style update API, for use with routers/devices with builtin DynDns support
- supports IPv4 and IPv6

## Deployment

### DNS

You'll need two different domains, such as `a.ddns.example.net` and `aaaa.ddns.example.net`, set up to point to the same server. One (`a.ddns.example.net`) should have only an A record pointing to the server's IPv4 address; and the other (`aaaa.ddns.example.net`) should have only an AAAA record pointing to the server's IPv6 address.

### Server & Client (systemd)

Create a user and group for the service to use, and create a directory in `/etc` for configuration:

```shell script
# as root...

useradd -r -U -s /sbin/nologin do-ddns
mkdir /etc/do-ddns
chown root:do-ddns /etc/do-ddns
```

### Server (systemd)

```shell script
# as root...

# update the version and OS/architecture in the following release URL as desired.
curl -L --silent https://github.com/cdzombak/do-ddns/releases/download/v0.0.1/do-ddns-0.0.1-linux_amd64.tar.gz | tar -C /usr/local/bin -xzv ./do-ddns-server
chown do-ddns:do-ddns /usr/local/bin/do-ddns-server
chmod +x /usr/local/bin/do-ddns-server

curl -L --silent https://raw.githubusercontent.com/cdzombak/do-ddns/master/server/deployment/.env.sample > /etc/do-ddns/.env
nano /etc/do-ddns/.env # customize as needed
chown root:do-ddns /etc/do-ddns/.env

curl -L --silent https://raw.githubusercontent.com/cdzombak/do-ddns/master/server/deployment/domains.json.sample > /etc/do-ddns/domains.json
nano /etc/do-ddns/domains.json # customize as needed
chown root:do-ddns /etc/do-ddns/domains.json

# Install and start the server systemd unit:
curl -L --silent https://raw.githubusercontent.com/cdzombak/do-ddns/master/server/deployment/do-ddns-server.service > /etc/systemd/system/do-ddns-server.service
systemctl daemon-reload
systemctl enable do-ddns-server
systemctl start do-ddns-server

# Test that the service is working:
curl -i localhost:7001/ping

# Configure nginx, or whatever frontend reverse proxy you're using:
curl -L --silent https://raw.githubusercontent.com/cdzombak/do-ddns/master/server/deployment/nginx-ddns.conf > /etc/nginx/sites-available/ddns.example.net
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

# update the version and OS/architecture in the following release URL as desired.
curl -L --silent https://github.com/cdzombak/do-ddns/releases/download/v0.0.1/do-ddns-0.0.1-linux_amd64.tar.gz | tar -C /usr/local/bin -xzv ./do-ddns-client
chown do-ddns:do-ddns /usr/local/bin/do-ddns-client
chmod +x /usr/local/bin/do-ddns-client

curl -L --silent https://raw.githubusercontent.com/cdzombak/do-ddns/master/client/deployment/.env.sample > /etc/do-ddns/.env
nano /etc/do-ddns/.env # customize as needed
chown root:do-ddns /etc/do-ddns/.env

# Install and start the client systemd unit:
curl -L --silent https://raw.githubusercontent.com/cdzombak/do-ddns/master/client/deployment/do-ddns-client.service > /etc/systemd/system/do-ddns-client.service
systemctl daemon-reload
systemctl enable do-ddns-client
systemctl start do-ddns-client
```

### Client (macOS)

```shell script
# update the version and OS/architecture in the following release URL as desired.
curl -L --silent https://github.com/cdzombak/do-ddns/releases/download/v0.0.1/do-ddns-0.0.1-darwin_amd64.tar.gz | tar -C /usr/local/bin -xzv ./do-ddns-client
chmod +x /usr/local/bin/do-ddns-client

curl -L --silent https://raw.githubusercontent.com/cdzombak/do-ddns/master/client/deployment/org.dzombak.do-ddns-client.plist ~/Library/LaunchAgents/org.dzombak.do-ddns-client.plist
sudo launchctl load -w ~/Library/LaunchAgents/org.dzombak.do-ddns-client.plist
```

## DynDns Update API Support

The server also supports clients which use the [DynDns update API](https://help.dyn.com/remote-access-api/perform-update/), like routers. Configuration required on the client:

- Hostname: the domain to update (eg. `home.example.net`)
- Username: the domain to update (eg. `home.example.net`)
- Password: the secret for the selected domain
- Server: the server running `do-ddns-server` (eg. `a.ddns.example.net`)

The DynDns API requires the client to pass its IP in the request. By default, `do-ddns-server` ignores this and uses the request's remote address. To allow using the IP passed by the client (in the `myip` query parameter), add `"allowClientIPChoice": true` to a domain's configuration.

If `allowClientIPChoice` is enabled, and the client's remote address as seen by the server is a different IP version from the `myip` passed by the client, the server will use both these pieces of information to update the domain's A and AAAA records.  

Note that the server does not allow updating multiple domains in one request, though the DynDns API does allow passing a comma-separated list of domains in the `hostname` field. `do-ddns-server` will return an `HTTP 400 Bad Request` in this case.

## Advanced Usage Notes

- Send the server process SIGUSR2 to reload its configuration file in-place.
- The domain configuration option `createMissingRecords` allows the server to create missing A/AAAA records for the domain as needed.

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
