# MyQ

[![GoDoc](https://godoc.org/github.com/joeshaw/myq?status.svg)](http://godoc.org/github.com/joeshaw/myq)

`myq` is a Go package and command-line tool providing access to
the Liftmaster / Chamberlain MyQ API.

With the MyQ API you can get a list of devices and open and close
garage doors and gates.

## Command-line tool

The `myq` tool can be installed with:

    go get github.com/joeshaw/myq/cmd/myq

Run `myq` by itself to see full usage information.

To list devices:

    myq -username <username> -password <password> devices

To open a door:

    myq -username <username> -password <password> open <device ID>

To close a door:

    myq -username <username> -password <password> open <device ID>

Usernames and passwords can also be provided through the environment
variables `MYQ_USERNAME` and `MYQ_PASSWORD`.

## MyQ protocol

David Pfeffer's [MyQ API reference on
Apiary](https://unofficialliftmastermyq.docs.apiary.io/) was a helpful
reference.

David also has an implementation in Ruby:
https://github.com/pfeffed/liftmaster_myq

ArrayLab has a Python implementation:
https://github.com/arraylabs/pymyq

J. Nunn has a Python implementation that ties in with Amazon Alexa:
https://github.com/jbnunn/Alexa-MyQGarage

## Contributing

Issues and pull requests are welcome.  When filing a PR, please make
sure the code has been run through `gofmt`.

## License

Copyright 2018 Joe Shaw

`myq` is licensed under the MIT License.  See the LICENSE file
for details.
