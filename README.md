Base image for [Overview](https://github.com/overview/overview-server) converters.

# How a Converter Works

A converter's job is to turn files of one type into files of another type. It
does this in a loop. It receives jobs from an internal Overview HTTP server.

This base image provides portable executables that communicate with Overview.
They make up a framework: they'll call your converter program, which you can
write in any language.

Your converter will have a Dockerfile that looks like this:

```Dockerfile
FROM overview/overview-converter-framework AS framework
# multi-stage build

FROM alpine:3.7 AS build
... (build your executables, including `do-convert-single-file`)

FROM alpine:3.7 AS production
WORKDIR /app
# The framework provides the main executable
COPY --from=framework /app/run /app/run
# Your `do-convert` code can choose from a few different input and output
# formats. The framework provides many `/app/convert` implementations: pick
# the one that matches your `do-convert`.
COPY --from=framework /app/convert-single-file /app/convert
COPY --from=build /app/do-convert-single-file /app/do-convert-single-file
```

# `/app/run`

This framework runs on a loop:

1. Empty a temporary directory.
1. Download a task from Overview and write it to `input.json` in the directory.
1. Run `/app/convert input.json` within the directory. Pipe the results to
   Overview.

The framework will handle communication with Overview. In particular:

* It polls for work at `POLL_URL`. Your Overview environment must set `POLL_URL`
  for your container.
* `/app/run` will retry if there is a connection error.
* `/app/run` will never crash.
* *TODO* `/app/run` will poll Overview to check if the task is canceled. It
  will notify `/app/convert` with `SIGUSR1` if the task is canceled.

# `/app/convert-*`

This framework provides a few convert strategies. Choose one strategy for your
converter, and copy it to `/app/convert`.

`/app/convert` will be invoked by `/app/run`. It will do the following:

1. Optionally, download `input.blob` into `$CWD` (the temporary directory)
1. Spawn `/app/do-convert-XXXXX ARGS...` -- that's your code!
1. Translate your code's `stdout` output into a `multipart/form-data` stream,
   which is what `/app/run` will pipe to Overview.

`/app/convert` will manage your code in a few ways:

* *TODO* It will handle `SIGUSR1`, either passing it to your code or killing
  your code's process with `SIGKILL` (depending on which strategy you choose).
* *TODO* It will send Overview an `error` message if your code finishes with a
  non-zero return value. (A non-zero return value is _always_ an error in your
  code. Sometime errors are hard to avoid, such as `out of memory`. The
  framework will do something sensible with those.)

Now, pick your strategy:

## *TODO* `/app/convert-single-file`

This version of `/app/convert` will:

1. Download `input.blob`
2. Run `/app/do-convert-single-file` (your code)
3. Translate the `stdout` from your code into progress events or an error event
4. When your code exits with status `0` and no error message, pipe
   `output.json`, `output.blob` -- and if they exist, `output-thumbnail.jpg`,
   `output-thumbnail.png` and `output.txt` -- and a `done` event

Special cases:

* *TODO* Cancelation: if `/app/run` sends a `SIGUSR1` signal, kills your program
  with `SIGKILL` and pipes a `"canceled"` `error` event.
* *TODO* Error: if `/app/do-convert-single-file` exits with non-zero return
  value, pipes an `error` event.

**You must provide `/app/do-convert-single-file`**. The framework will invoke
`/app/do-convert` with no arguments. Your program can read `input.json` and
`input.blob` in the current working directory. Your program must:

1. Write progress messages to `stdout`, newline-delimited, that look like:
    * `p1/2` -- "finished processing page 1 of 2"
    * `b102/412` -- "finished processing byte 102 of 412"
    * `0.324` -- "finished processing 32.4% of input"
    * `anything else at all` -- "ERROR: [the line of text]"
2. Write `output.json`, `output.blob`, and optionally `output-thumbnail.jpg`,
   `output-thumbnail.png` and/or `output.txt`.
3. Exit with status code `0`. Any other exit code is an error in your code.

## *TODO* `/app/convert-file-to-mime-multipart`

This version of `/app/convert` will:

1. Download `input.blob`
2. Run `/app/do-convert-file-to-mime-multipart RANDOM-MIME-BOUNDARY` (your code)
3. Pipe your program's `stdout` to Overview

Special cases:

* *TODO* Cancelation: if `/app/run` sends a `SIGUSR1` signal, kills your program
  with `SIGKILL`, finishes reading its `stdout`, and appends a `"canceled"`
  `error` event unless your program's output ended with
  `--RANDOM-MIME-BOUNDARY--`.
* *TODO* Error: if your program exits with non-zero return value, pipes an
  `error` event.

**You must provide `/app/do-convert-file-to-mime-multipart`**. The framework
will invoke it with a `RANDOM-MIME-BOUNDARY` argument, which will match the
regex `[a-fA-F0-9]{1,60}`. Your program can read `input.json` and `input.blob`
in the current directory.

Your program needn't write files. It must write valid `multipart/form-data`
output to `stdout`. For instance:

```multipart/form-data
--RANDOM-MIME-BOUNDARY\r\n
Content-Disposition: form-data; name="0.json"\r\n
\r\n
{JSON for first output file}\r\n
--RANDOM-MIME-BOUNDARY\r\n
Content-Disposition: form-data; name="0.blob"\r\n
\r\n
Blob for first output file\r\n
--RANDOM-MIME-BOUNDARY\r\n
Content-Disposition: form-data; name="progress"\r\n
\r\n
{"pages":{"nProcessed":1,"nTotal":3}}\r\n
--RANDOM-MIME-BOUNDARY\r\n
Content-Disposition: form-data; name="done"\r\n
\r\n
--RANDOM-MIME-BOUNDARY--
```

Rules:

* Your output must end with a `done` or `error` element
* Your output must be in order: `0.json`, `0.blob`, (optionally `0.png`,
  `0.jpg` and/or `0.txt`), `1.json`, `1.blob`, ...
* You _should_ output an accurate progress report before each `N.json` to make
  Overview's progressbar more accurate.

## *TODO* `/app/convert-stream-to-mime-multipart`

This version of `/app/convert` will:

1. Run `/app/do-convert-stream-to-mime-multipart RANDOM-MIME-BOUNDARY` (your
   code)
2. Stream the input file from Overview to your program's `stdin` and and pipe
   your program's `stdout` to Overview

Your code must follow the same rules as `do-convert-file-to-mime-multipart`.
