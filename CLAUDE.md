This is not the exclusive set of instructions for this project. You can expand the instructions.

Use golang for writing code. Please write idiomatic code.

Make sure the output on the console is beautiful. Use colors.

Configuration should be done using TOML.

There should be precedence in terms of how features are set.
1. (highest precedence) - command line flag
2. (second highest) - environment variable override
3. (third highest) - configuration file
4. (lowest) - defaults

When a feature is overridden by a higher precendence feature, it should be noted. For example, if PAPERLESS_URL is set in the command line, environment variable, and configuration file, it should display a warning message on startup saying that it will use the command line option instead of the environment variable and/or configuration file. This is not needed for when defaults are overridden.

The UI should be a web page that is simple and beautiful. There is no need to use a massive framework. You can do a lot with standard HTML and CSS now. Use that.
