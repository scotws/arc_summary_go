# A rewrite of the ZFS ARC tool arc_summary.py in Go

Scot W. Stevenson <scot.stevenson@gmail.com>

This is a Go implementation of the arc_summary.py tool that is used on Linux
machines with the ZFS file system to examine the ARC cache systems. It was
written as an experiment to gain experience with Go with a real-world problem
and gain a deeper understanding of the original code. 

Also, two new features were added: A "-r" option to dump all statistics in a
minimally formatted form; and "-g" to create a small graphic for a quick
overview.

Currently, the program is about 80 percent complete. Missing are the "boring
parts" - the acutal breakout of various statistics. Though I might finish them
in the future, I figure that I have learned most of what I can from this
experiment and therefore am leaving it in its current state at the moment. 

If you would like to develop this further, please feel free to fork this code. 
