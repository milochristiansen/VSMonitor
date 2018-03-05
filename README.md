
Vintage Story Server Monitor and Remote Console
=======================================================================================================================

This package includes a simple server monitor and remote console for Vintage Story servers.

Currently this is quite basic, offering a simple web UI that allows running server commands and a small set of monitor
commands.


Setup:
-----------------------------------------------------------------------------------------------------------------------

Setup is non-trivial. Not hard, but not simple either.

Unpack the package into your Vintage Story directory for simplicity. You could unpack it elsewhere, but that would
require more configuration.

Next you need to start the monitor. Do not set any commandline options just yet. As soon as the monitor is up (which will
be right away) open `https://localhost` in your web browser. The first thing you need to do is create an account for
yourself.

In the input field at the top of the page, type `:token issue <username>`, replacing `<username>` with whatever you want.
This should print a token to the text area below the input box. Copy this token, then click "Settings" below the input box
paste your new token into the "Token" field and click "OK".

Finally, enter `:token revoke root` into the input box to remove the setup credentials and type `:kill monitor` to shut down.

Now you are ready to open the monitor to the internet.

Start the monitor with the following command line options (where `<username>` is the username you used before):

	vs_monitor -admin="<username>" -host="<your domain name>"

On Linux or OSX you may need to add `-bin="mono VintagestoryServer.exe"`, and you will need to run the monitor as root. Once
the monitor is up, open the monitor UI in your web browser and type `:recover` to start the game server. If the monitor
complains about an invalid token you need to set your token in the settings again. This will happen if you use the proper
domain name to connect and is expected.

If you are connecting from a remote system you will need to copy over your token or issue a new one for that system, as well
as setting the server address in the web UI settings page.

Note: If there are services already using TCP ports 80 and 443, you will not be able to use automatic certificates. In
this case provide the `-nocert` command line option, and put your certificate in `monitor/cert.key` and `monitor/cert.crt`.


When the Server Goes Down
-----------------------------------------------------------------------------------------------------------------------

If something happens to your Vintage Story server and it crashes, don't worry! The monitor will restart it no problem.

If it crashes again immediately, the monitor will just start it again unless it crashes 3 times in 15 seconds, in which
case it will stop trying and wait for you.

Once you fix whatever the problem was, simply tell the monitor to `:recover` and it will relaunch the game server.

If you want to restart the server, tell it to `/stop` via the monitor, then when it has shut down tell the monitor to
`:recover` and the server will start back up.

If your server hangs and won't listen to commands, you can tell the monitor to `:kill server` and it will force it to
shut down (hopefully).
