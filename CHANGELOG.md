## v0.1.0 - 2020-05-11

* `test-convert-single-file`: Use QPDF to compare PDFs.
    * Output legible diffs on error
    * Require consistent /CreationDate (converters should be deterministic)
    * /IDs are still allowed to differ. A future release may change this.
