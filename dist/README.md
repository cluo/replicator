# Dist

The `dist` folder contains sample configs for various platforms.

## Assumptions

The examples assume you will either be using replicators default settings or that you will be writing your configuration parameters to `/etc/replicator.d/replicator.hcl` on the filesystem.

## Upstart

On systems using [upstart](http://upstart.ubuntu.com/) the basic upstart file under `upstart/replicator.conf` starts and stops the replicator binary and should be placed under `/etc/init/replicator.conf`. This will then allow you to control the daemon through `start|stop|restart` commands.

## Systemd

On systems using [systemd](https://www.freedesktop.org/wiki/Software/systemd/) the basic systemd unit file under `systemd/replicator.service` starts and stops the replicator daemon and should be placed under `/etc/systemd/system/replicator.service`. This then allows you to control the daemon through `start|stop|restart` commands.
