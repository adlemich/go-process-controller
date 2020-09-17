# go-process-controller
A simple implementation to start, monitor, restart and stop processes as defined from a config file.

This has been tested under Windows only. Since it uses some Windows specifics (e.g. killing processes), it will certainly only work under Windows as tested.

Features
 - Allows to write an example configuration file (JSON) with correct structure
 - Reads from a configuration file about which processes it shall start and monitor
 - Logging with rotating logs, and configurable max file size
 - Launching and monitoring processes
    - Run and wait for it to finish with timeout
    - Run without window (hidden)
    - Redirect stdout and stderr to logiles
    - allow to restart a process if it terminates with max retries



