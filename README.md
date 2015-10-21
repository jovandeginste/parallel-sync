# parallel-sync

This tool will eventually be able to synchronize content of one directory to another directory in parallel; parallel meaning multiple files simultaneously, not multiple thread per file.

I started working on this tool after having sync'ed several times directories from GPFS to other locations, containing several TBs of files of different sizes.

Issues I'm trying to solve eventually:
* some directory contains several tens of thousands of files, whose names differ hardly, many of the same (small) size
* directory many levels deep contains some tree of dirs and files with a large set of the data
* the content is totally mixed, file sizes range from bytes to several gigs
* ...

Eventual target would be to have several components:

1. create metadata databases
  * list of source metadata
  * list of destination metadata

2. create tasklist based on metadata
  * compare source-destination metadata, create tasks with priority

3. process tasks (in parallel)
  * issue: directory metadata (changes when creating files)
  * solution: create two tasks for directory
    * create directory (high priority)
    * update metadata (low priority)
