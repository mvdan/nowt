#!/bin/bash

tdir=$(mktemp -d)
pkg=test-cache-open-modtime

reset_mtime=false

clone_and_test() {
	dir=$tdir/nowt

	git clone --quiet --depth=1 https://github.com/mvdan/nowt $dir

	pushd $dir >/dev/null
	if $reset_mtime; then
		touch -d '2000-01-01 00:00:00' $pkg/testdata/foo.txt
	fi
	ls -lah $pkg/testdata/foo.txt
	go test -trimpath ./$pkg

	popd >/dev/null
	rm -rf $dir
}

echo "Clone and test twice in a row, using the same directory and -trimpath. The test caching does not work."
clone_and_test
echo
clone_and_test
echo

reset_mtime=true

echo "The same again, but this time resetting the testdata file's mtime, since git clone otherwise sets it to the time it ran."
clone_and_test
echo
clone_and_test
echo

rm -rf $tdir
