plist.go
========

This is a go encoding for Apple style .plist xml files. This is derived from plist.pl from
Russ Cox, but extended to support the real, date, data and boolean plist types. I also
took out a bit of the error checking for basic types, this allows to postpone the type
check to reflect and thus use []interface{} as the base type for an array.
