
Vintage Story Server Monitor and Remote Console
=======================================================================================================================

This package includes a simple server monitor and remote console for Vintage Story servers.

Currently this is quite basic, offering a simple web UI that allows running server commands and a small set of monitor
commands.


Building:
-----------------------------------------------------------------------------------------------------------------------

Building is easy. First make sure you have a recent version of Go installed, then run:

	go get github.com/milochristiansen/VSMonitor

Change to the directory you want your servers in, and then run:

	go build VSMonitor

If this is the first time you have built the monitor in a given directory, run it once to generate a default environment
and config:

	./VSMonitor

Now download the contents of the `ui` directory in the repository to `./Monitor/ui` (where `.` is where you put the
`VSMonitor` binary).


Configuring:
-----------------------------------------------------------------------------------------------------------------------

Once you have a binary in place and ready to run, simply start it to generate the default configuration. This will create
a few required directories alongside the binary, and create a default config file for you to edit (`./Monitor/cfg.json`).

If at all possible you want to set the `HostName` key to the domain name of your server, then set `AutoTLS` to `true`.
This will allow your server to automatically get the SSL certificates it needs from [Let's Encrypt](https://letsencrypt.org/).
This requires you to run the monitor as root and makes it use TCP ports 80 and 443. If you cannot satisfy these
requirements, you will need to set `Port` and put your SSL certificate in `./Monitor/cert.key` and `./Monitor/cert.crt`.

You cannot run the monitor without SSL! Well, you can, but only if you set `HostName` to `localhost`, in which case you
can only get to the monitor UI from the same machine it is running on (this is intended for local testing).

Once you have your connection settings finished, go ahead and start the monitor, then open its web UI in your browser.
Your first order of business is creating a administrator account. To do this, simply enter `:user create "your name"`
in the input box near the bottom of the page. The first account created is automatically the administrator. The monitor
should give you a "token". Copy this token and then open the settings window (button in bottom left corner). You should
paste your token in the settings window, then click "OK".

You can add more users in the same way.

Now to create a server. For this example we will make a new server with the latest stable version. All you need to do
is enter `:server create "Example Server" stable`, then what while the monitor downloads the required files (it only
needs to do this once for any given version, the files are shared by multiple servers if you create them). Once it is
finished you will get a message telling you the server is installed, and the UI will open a tab for your new server.
From the new tab, simply run `:recover` to start the server.

To update your server, simply `/stop` it and `:server update`, then `:recover` when the monitor is done downloading
the new version.

Running multiple servers is a bit harder, since each server needs its own port. You will need to edit the server's
configuration file. By default this will be in `./GameData/<server name> <SID>` where `<server name` is the name you
specified when you created the server, and `<SID>` is a unique server ID number.


When the Server Goes Down
-----------------------------------------------------------------------------------------------------------------------

If something happens to your Vintage Story server and it crashes, don't worry! The monitor will restart it no problem.

If it crashes again immediately, the monitor will just start it again unless it crashes 3 times in 60 seconds, in which
case it will stop trying and wait for you.

Once you fix whatever the problem was, simply tell the monitor to `:recover` and it will relaunch the game server.

If you want to restart the server, tell it to `/stop` via the monitor, then when it has shut down tell the monitor to
`:recover` and the server will start back up.

If your server hangs and won't listen to commands, you can tell the monitor to `:kill server` and it will force it to
shut down (hopefully).

If you want to shut the monitor down just `:kill monitor`, this *should* stop the game server(s) too, but it is strongly
recommended you `/stop` them first so as to have no chance of "orphan" servers.


Monitor API
-----------------------------------------------------------------------------------------------------------------------

The monitor provides two main endpoints:

* `/ui`: This is a simple web server providing static files for the web UI. A convenience mostly.
* `/socket`: A web socket connection for interacting with the monitor.

The web socket is the important one. This connection is pretty simple, it is JSON both ways, and always the same structure.

Messages from the monitor use the following format:

	{
		"SID": 0,
		"At": "2006-01-02T15:04:05Z07:00",
		"Class": "Monitor",
		"Message": "Example log message."
	}

* `SID`: The Server ID, this number tells you which server is talking to you (or which server you were trying to talk
  to in the case of messages from the monitor). SID 0 is used for monitor commands only, it is not backed by an actual
  server.
* `AT`: A RFC3339 formatted message timestamp.
* `Class`: The log message class. Most are from the game, but `"Monitor"`, `"Monitor Error"`, and `"Monitor Init"` are
  used for messages from the monitor.
* `Message`: The log message being reported.

Messages to the monitor must use the following format:

	{
		"SID": 0,
		"Token": "DEADBEEFDEADBEEFDEADBEEFDEADBEEF",
		"Command": "/example"
	}

* `SID`: The Server ID, this number tells the monitor which server you want to talk to. SID 0 is used for monitor
  commands only, it is not backed by an actual server.
* `Token`: Your 32 character hexedecimal API token.
* `Command`: The command you want to run.

When you connect to the the monitor, the client will receive a message for each server available. This message will have
the SID, current time, the class `"MonitorInit"`, and the server name as the payload. Use these messages to handle any
initialization you need to do. After the init messages you will receive the full log flow from all active servers. If
a new server spins up, you will get an init message for it, followed by log messages.
