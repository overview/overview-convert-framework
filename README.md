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

1. Download a task from Overview as JSON.
1. Open a stream to download the body of the input file.
1. Stream the body to `/app/convert MIME-BOUNDARY JSON` and pipe the results to
   Overview.

`/app/run` handles all communication with Overview. In particular:

* `/app/run` polls for tasks at `POLL_URL`. Overview's administrator must set
  `POLL_URL` for your container.
* `/app/run` will retry if there is a connection error.
* `/app/run` will never crash.
* *TODO* `/app/run` will poll Overview to check if the task is canceled. It
  will notify `/app/convert` with `SIGINT` if the task is canceled.

# `/app/convert` -- a.k.a., `/app/convert-*`

`/app/convert` is a program we provide, under a few different names. That is,
when you create your program you'll choose one of the following implementations
to copy into `/app/convert` in your image.

From `/app/run`'s point of view, `/app/convert` will read the input stream
and `JSON` command-line argument and produce a `multipart/form-data` output
stream with MIME boundary `MIME-BOUNDARY` (in C lingo, `argv[1]`).
`/app/convert` will never crash, and it will always output a data stream that
Overview can handle.

Your code is invoked by `/app/convert`, following one of these strategies:

## `/app/convert-single-file`

This version of `/app/convert` will:

1. Write standard input to `input.blob` in a temporary directory and verify it's
   the correct size
1. Run `/app/do-convert-single-file JSON` (*your code*) in the temporary
   directory
1. Translate the `stdout` from your code into progress events or an error event
1. When your code exits with status `0` and no error message, pipe
   `output.json`, `output.blob` -- and if they exist, `output-thumbnail.jpg`,
   `output-thumbnail.png` and `output.txt` -- and a `done` event

Special cases:

* Cancelation: if `/app/run` sends a `SIGINT` signal, kills your program with
  `SIGKILL`.
* Error: if `/app/do-convert-single-file` exits with non-zero return value,
  pipes an `error` event.

**You must provide `/app/do-convert-single-file`**. The framework will invoke
`/app/do-convert JSON`. Your program can read `input.blob` in the current
working directory. Your program must:

1. Write progress messages to `stdout`, newline-delimited, that look like:
    * `p1/2` -- "finished processing page 1 of 2"
    * `b102/412` -- "finished processing byte 102 of 412"
    * `0.324` -- "finished processing 32.4% of input"
    * `anything else at all` -- "ERROR: [the line of text]"
1. Write `output.json`, `output.blob`, and optionally `output-thumbnail.jpg`,
   `output-thumbnail.png` and/or `output.txt`.
1. Exit with status code `0`. Any other exit code is an error in your code.

## *TODO* `/app/convert-stream-to-mime-multipart`

This version of `/app/convert` will:

1. Create an empty temporary directory
1. Run `/app/do-convert-stream-to-mime-multipart MIME-BOUNDARY JSON` (*your
   code*) within the temporary directory
1. Stream the input file from Overview to your program's `stdin` and and pipe
   your program's `stdout` to Overview

Special cases:

* *TODO* Cancelation: if `/app/run` sends a `SIGINT` signal, kills your program
  with `SIGKILL`.
* *TODO* Error: if your program exits with non-zero return value, pipes an
  `error` event.
* *TODO* Buggy code: emits an `error` event if your program does not produce a
  `error` or `done` event or end with `--MIME-BOUNDARY--`.
* *TODO* Temporary files: if your program emits temporary files to its current
  working directory, they will be deleted.

**You must provide `/app/do-convert-stream-to-mime-multipart`**. The framework
will invoke it with `MIME-BOUNDARY` and `JSON` as arguments. `MIME-BOUNDARY`
will match the regex `[a-fA-F0-9]{1,60}`. Your program can read `input.blob`
in the current directory.

Your program must write valid `multipart/form-data` output to `stdout`. For
instance:

```multipart/form-data
--MIME-BOUNDARY\r\n
Content-Disposition: form-data; name="0.json"\r\n
\r\n
{JSON for first output file}\r\n
--MIME-BOUNDARY\r\n
Content-Disposition: form-data; name="0.blob"\r\n
\r\n
Blob for first output file\r\n
--MIME-BOUNDARY\r\n
Content-Disposition: form-data; name="progress"\r\n
\r\n
{"pages":{"nProcessed":1,"nTotal":3}}\r\n
--MIME-BOUNDARY\r\n
Content-Disposition: form-data; name="done"\r\n
\r\n
--MIME-BOUNDARY--
```

Rules:

* Your output must end with a `done` or `error` element. A `done` element
  should be empty; an `error` element must include an error message.
* Your output must be in order: `0.json`, `0.blob`, (optionally `0.png`,
  `0.jpg` and/or `0.txt`), `1.json`, `1.blob`, ..., `done`.
* You _should_ output an accurate progress report before each `N.json` to help
  Overview's progressbar behave well.

## Roll your own

Even more lightweight than `/app/convert-stream-to-mime-multipart` is to roll
your own version of `/app/convert`. Beware, though:

* Your own version of `/app/convert` must always output messages to Overview:
  especially a `done` or `error` event. Without those events, Overview will
  never finish processing the file: it will retry indefinitely.
* Your own version of `/app/convert` must always exit successfully. The
  trickiest case, in our experience, is handling "out of memory." If your
  `/app/convert` does not exit successfully, Overview will retry indefinitely
  and the file will never be processed.
* Your own version of `/app/convert` should output helpful error messages, so
  you can debug it easily.
* Your own version of `/app/convert` should end quickly after receiving
  `SIGUSR`, because Overview will ignore all further output.
* Your own version of `/app/convert` must ensure temporary files invoked during
  one invocation aren't read by the next invocation: that would leak users'
  documents to other users.

`/app/convert-stream-to-mime-multipart` is small and fast, and it solves these
problems for you. You probably want it.

# To Maintain This Repository

## Coding

`./dev` will start a development loop that runs tests. Restart it if you edit
`Dockerfile`.

## Testing

`docker build .` will run all tests.

Tests are in `./test/*/suite.bats`. They're run in
[bats](https://github.com/sstephenson/bats), an ideal framework for testing
programs that pipe data around.

## Releasing

`./release MAJOR.MINOR.PATCH` will push to GitHub. Docker Hub will build the
images for mass consumption.

# License

This software is Copyright 2011-2018 Jonathan Stray, and distributed under the
terms of the GNU Affero General Public License. See the LICENSE file for details.
