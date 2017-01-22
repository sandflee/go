---
title: k8s on spark调研
date: 2017-01-02
categories: k8s,spark
tags: [k8s,spark]
---


# spark on k8s调研
目前k8s只支持以standalone模式运行spark集群，standalone模式功能比较简单，很少用于生产环境。本文档调研其他支持方式。

## 背景
### spark 原理
spark程序将用户任务拆分成一系列task(DAG)，分发给work执行。
1. driver 类似与app的master，会对用户程序进行拆分成task，向cluster请求资源
2. executor 工作进程，负责执行具体的task
3. app 概念上对应用户的一个应用，实现上对应一个sparkContext对象
4. job 一个RDD的action动作触发一个job
5. stage  一个job可能被划分为多个stage,划分依据为是否需要full data shuffle.一个stage包含一系列task
6. task 执行任务的最小单元，每个task会处理一片数据 
7. client mode/cluster mode  client mode指driver在提交任务的client运行，方便调试，cluster mode指driver在cluster中运行，用于生产。
8. Dynamic Allocation.  driver程序会根据task的繁忙情况自动增加/减少 executor数目

### spark on yarn 原理
spark以plugin的方式提供cluster功能的扩展，需要实线资源请求接口，executor启动接口等。
以cluster mode为例，spark app基本等价于yarn app。
1. client将相关jar上传到hdfs，向RM请求启动spark am
2. spark am执行driver相关逻辑，拆分task，请求资源，请求executor,分发task
3. NM上启动executor,并接受driver分发的task
4. executor执行task
5. 如果没有开启Dynamic Allocation， executor提供shuffle功能，executor在app结束时才会回收，造成资源浪费。如果开启，需要一个外部的shuffle service来提供shuffle功能（NM aux sericce里配置sparkShuffleService）。
6. task执行shuffle操作等，最终任务结束

### 数平运营spark现状
大部分spark作业在独立的spark集群运行。通过gaia队列机制为BG/用户提供资源隔离

## k8s native方式支持spark

社区已经有相关讨论：
spark： https://issues.apache.org/jira/browse/SPARK-18278
k8s： https://github.com/kubernetes/kubernetes/issues/34377

### 实现思路：
1. 用户提交driver pod, driver pod负责向k8s请求资源，得到资源后创建executor pod
2. driver和executor的交互，driver对app的拆分都属于spark内部罗辑无需改动
3. gaia用的话可能还要封装一层，把driver和executor pod作为一个app看待

### 问题：
1. 如果开启Dynamic Allocation，需要提供额外shuffle service，目前倾向于deamonSet方式实现
2. client mode， driver运行在cluster外部，executor会向driver注册，网络不通
3. cluster mode, resource下载问题。yarn nm会自动下载资源，k8s没有相应机制。executor自己下载资源？ 利用pod prestart接口？
4. cluster mode，下non-jvm binding，不太明白具体什么意思
5. 多用户资源管理。目前k8s不如yarn强大，不能共享集群资源
6. 认证鉴权
7. 数据本地性  spark申请资源时会优先申请HDFS数据所在机器，k8s缺乏类似机制

### 总结
以native方式把spark跑起来，感觉不是很困难，但后面的稳定性以及跟原有yarn方式对比是否对用户更易用，可能还有一段距离
spark社区对是否支持k8s也不是很积极。