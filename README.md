# Task Manager 98

Rewrite of the classic Windows 98, NT4 and Windows XP Task Manager for Windows 11 (arm64 and x86_64). It is rewritten based on and has feature parity with Task Manager Version 5.1 (Build 2600.xpsp_sp2_rtm.040803-2158) from 2001. Task Manager 98 is written in Go and only uses Windows system libraries for the User Interface and all its functionality. The original Task Manager 5.1 was written in C++ by Microsoft. The rewrite in Go ensures better memory safety while keeping very good performance as a self-contained binary.

## Screenshots

![Applications](doc/applist.jpg)
![Processes](doc/proclist.jpg)
![Performance](doc/perfview.jpg)
![Networking](doc/netview.jpg)

## Status

The application is work in progress, but the `master` branch should always compile for Windows 11 on arm64 and x86_64. 

## License

This software is published under a **Source-Available** License.

```
Copyright (c) 2026 Jan Kammerath. All rights reserved.

Permission is hereby granted to view, inspect, and evaluate the source code of 
this software for personal, non-commercial, and educational purposes. You may compile 
and run the software locally on your own devices solely for personal use.

Redistribution, republishing, sublicensing, modification for public distribution, 
or making the software (in source or binary form) available to third parties—including, 
but not limited to, distribution via websites, package managers, app stores, or digital 
marketplaces—is strictly prohibited without prior written permission from the copyright holder.

Commercial use, commercial redistribution, or resale in any form is strictly prohibited.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND...
```