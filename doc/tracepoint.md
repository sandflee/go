---
title: 利用tracepoint查看系统调用相关信息
date: 2016-12-20
categories: tools
tags: [linux, tool]
---

##  利用tracepoint查看系统调用相关信息
线上机器出现大量socket TIME_WAIT,netstat 发现本地有进程一直在连接container export端口.但netstat lsof都找不到哪个在连接.
zhiguo介绍了tracepoint工具.
### step
1. mount发现debugfs已经mount debugfs on /sys/kernel/debug type debugfs (rw,relatime)
2. 查看是否支持connect,  cat /sys/kernel/debug/tracing/available_events | grep connect  --> syscalls:sys_exit_connect
syscalls:sys_enter_connect
3. 开启connect的trace, echo 1 > /sys/kernel/debug/tracing/events/syscalls/sys_enter_connect/enable
4. 查看output, cat /sys/kernel/debug/tracing/trace, 发现haproxy一直在connect,进行健康探测
5. 关闭trace,echo 0 > /sys/kernel/debug/tracing/events/syscalls/sys_enter_connect/enable

### other
1. tracepoint基于内核kprobe机制, 基本原理在调用时hook
2. systemtap 提供了更强大的可编程支持


https://www.kernel.org/doc/Documentation/trace/tracepoints.txt
http://blog.csdn.net/trochiluses/article/details/10185951t