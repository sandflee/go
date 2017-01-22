---
title: storm-on-k8s
date: 2017-01-22
categories: k8s,storm
tags: [k8s,storm]
---

storm是目前主流的分布式流计算引擎，用clojure开发，很多人觉得不方便维护和二次开发，数平和阿里用java对其重构，开发出了jstorm，但基本概念和原理一致。
## 基本概念
- topology    类似与app，用户可以指定spout和bolt的拓扑关系，woker，task数目等
- spout    不断产生新的tuple，交给下游bolt处理
- bolt    处理tuple，并可能产生新的tuple，交给下游bolt处理
- tuple  包含一系列value，storm中数据抽象的最小单位？

## 运行时模块：
- nimbus,   storm中的master节点，负责把topology切分不同的task，并分发到每台机器（通过写入zk），nimbus本身没有状态，数据持久化到zk
- supervisor,   每台机器上的agent节点，负责监听zk，接收nimbus分配的task,启动worker运行。
 - worker  一个jvm进程，一个worker只运行一个topology任务的子集。 一个worker包含一个或多个executor。
 - executor   每个executor只会运行1个topology的1个component(spout或bolt)的task,  一个executor可能会运行多个task，但只有一个运行线程。
 - task      task是最终运行spout或bolt中代码的单元. 在Topology的生命周期中，一个Component的Task个数总是保持不变的，但是Component的Executor线程数却是可以改变的
- UI，查看集群运行状况 

## bolt之间数据route方式
- 随机传给下一个bolt
- 传给指定bolt
- 根据某些field hash
- 用户指定
- 等等

## task之间数据底层传输方式

- Intra-worker communication in Storm (inter-thread on the same Storm node): LMAX Disruptor
- Inter-worker communication (node-to-node across the network): ZeroMQ or Netty
- Inter-topology communication: nothing built into Storm, you must take care of this yourself with e.g. a messaging system such as Kafka/RabbitMQ, a database

## storm on yarn
storm on yarn目前只是在AM中把nimbus启动起来，再启动一些supervisor,只是启动集群的部署作用。（不需要yarn的队列隔离功能？）
spark on yarn会为每个app创建一个AM，对app资源进行管理，跟yarn更耦合。

## storm on k8s example问题
1, nimbus ha， nimbus本身无状态，用rc控制，保证系统中最终只有一个。但在比较短的时间内会有两个nimbus运行，对storm集群有什么影响？ 或者跟目前nimbus ha一样运行两个nimbus进程？需要跟storm团队再确认下。
2, storm资源管理方式和k8s的结合s
 - 目前storm还是采用memory slot,cpu slot的方式来分配内存，对于worker没有有效的资源控制（jvm -Xmx控制不住），裸机跑可能没问题，但在k8s上跑可能会被kill掉。
 - 一台机器的资源交给一个supervisor管理还是按需启动多个？

## 总结：
目前storm比较成熟的部署方式还是裸机部署，nimbus已经足够强大稳定，没有像spark一样借助yarn mesos做资源管理。在k8s上运行跟yarn上差别不大。没有特别大的问题。