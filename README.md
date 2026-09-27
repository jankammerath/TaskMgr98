# Task Manager 98

Rewrite of the classic Windows 98, NT4 and Windows XP Task Manager for Windows 11 (arm64 and x86_64). It is rewritten based on and has feature parity with Task Manager Version 5.1 (Build 2600.xpsp_sp2_rtm.040803-2158) from 2001. Task Manager 98 is written in Go and only uses Windows system libraries for the User Interface and all its functionality. The original Task Manager 5.1 was written in C++ by Microsoft. The rewrite in Go ensures better memory safety while keeping very good performance as a self-contained binary.

## Screenshots

![Applications](doc/applist.jpg)
![Processes](doc/proclist.jpg)
![Performance](doc/perfview.jpg)
![Networking](doc/netview.jpg)

## License

This software is published under the [Mozilla Public License Version 2.0](https://www.mozilla.org/en-US/MPL/).