---
title: HDFS笔记
date: 2016-11-26 
categories: hadoop
tags: [hadoop,hdfs,存储]
---

## HDFS笔记
> HDFS is designed more for batch processing rather than interactive use by users. The emphasis is on high throughput of data access rather than low latency of data access

### 数据模型：
- write once，read many times， 不支持在中间位置写。适合mr批数据处理
- 采用名字空间和数据分离的架构，名字空间由namenode维护，数据由datanode维护。提升系统的可扩展性。

### 模块：
#### namenode
nn的元数据由名字空间，文件和block的映射关系（image和edit log的形式持久化），block和datanode的映射关系（datanode动态保存）等组成。

### datanode
负责具体的数据存储，维护了block的具体信息

### journalnode
为namenode edit log提供持久化存储
### 数据流：

#### client
##### DistributeFileSystem

提供了对用户的操作接口，create open mkdir remove等，具体跟name node的交互在DFSClient实现。create后返回DFSOutputstream， open返回DFSInputStream
##### DFSOutputstream
1. write时先写在本地buf(FSOutputSummer),buf满了再调用DFSOutputStream#writeChunk
2. writeChunk 将数据打包成packet交给DateStreamer处理
3. DateStreamer 将数据保存再dataQueue,后台线程负责发送
4. DateStreamer利用DFSClient向name node请求block locate，以pipeline方式发送

##### DFSInputStream
从namenode取出相应block location，进行读取
##### DFSClient
负责跟namenode协议交互，具体参考ClientProtocol

### 模块协议：
#### ClientProtocol
包括create,mkdir, delete等基本的文件接口
getBlockLocations（对应open接口）, addBlock（write完成后通过这个接口获取block location信息，实现上将其作为前一个blocks的commit信息），renewlease等hdfs特有接口

#### DataNodeProtocol
1. registerDataNode  注册datanode
2. sendHeartBeart   定期heartbeat，同时作为namenode向datanode发送命令的一个渠道
3. BlockReport  datanode向namenode汇报所有block信息
4. BlockReceviedAndDeleted  datanode收受新的block或者delete block后向namenode发送

#### DataTransferProtocol
具体数据交互。发送实现 sender(client,nn,dn会调用) 接收serverXceiver（dn实现）
1. readBlock 读取block信息
2. writeBlock   pipeline write时调用此接口
3. transferBlock  把一个block copy到其他datanode？ balancer会用到

#### namenode
nameNodeRpcServer 实现所有rpc协议，proxy给具体的server处理
namenodeprotocol   dataNode和nameNode通信的唯一通道，registerNode和heartBeat. FSNameSystem.BlockManager处理
ClientProtocol    client和namespace通信的通道。FSNameSystem处理

todo： namenode datanode 数据交互流程的实现



### 机制：
#### replica 放置

不把在机器上均衡资源作为自己的目标。依靠后面的balancer来实现
本地磁盘满了，还能在本地放置数据吗？
> The purpose of a rack-aware replica placement policy is to improve data reliability, availability, and network bandwidth utilization

> For the common case, when the replication factor is three, HDFS’s placement policy is to put one replica on one node in the local rack, another on a different node in the local rack, and the last on a different node in a different rack. This policy cuts the inter-rack write traffic which generally improves write performance. The chance of rack failure is far less than that of node failure; this policy does not impact data reliability and availability guarantees. However, it does reduce the aggregate network bandwidth used when reading data since a block is placed in only two unique racks rather than three. With this policy, the replicas of a file do not evenly distribute across the racks. One third of replicas are on one node, two thirds of replicas are on one rack, and the other third are evenly distributed across the remaining racks. This policy improves write performance without compromising data reliability or read performance.

#### namenode ha
分为active nn和backup nn，active nn负责实际的数据处理，并把edit log写在journal node，backup从journal node读取edit log维护最新的状态，并定期作checkpoint
datanode 连接不上active时尝试连接standby nn
每台机器上可以部署zkfc进程进行自动failover

#### lease机制
保证only one writer。

#### safe mode

> During start up the NameNode loads the file system state from the fsimage and the edits log file. It then waits for DataNodes to report their blocks so that it does not prematurely start replicating the blocks though enough replicas already exist in the cluster. During this time NameNode stays in Safemode. Safemode for the NameNode is essentially a read-only mode for the HDFS cluster, where it does not allow any modifications to file system or blocks. Normally the NameNode leaves Safemode automatically after the DataNodes have reported that most file system blocks are available. If required, HDFS could be placed in Safemode explicitly usingbin/hadoop dfsadmin -safemode command. NameNode front page shows whether Safemode is on or off. A more detailed description and configuration is maintained as JavaDoc for setSafeMode().

#### why pipeline write？
最小化集群网络开销。如果都在client机器write，对client机器压力比较大。

#### 为什么不保存机器和block的映射关系？
We initially attempted to keep chunk location information persistently at the master, but we decided that it was much simpler to request the data from chunkservers at startup, and periodically thereafter. This eliminated the problem of keeping the master and chunkservers in sync as chunkservers join and leave the cluster, change names, fail, restart, and so on


### 问题：
1. 批处理任务启动时，几百个客户端同时读取文件.可以人工调节文件replica

2. 程序启动时，同时操作hdfs，本地缓存导致内存暴增。NM


http://itm-vm.shidler.hawaii.edu/HDFS/ArchDocCommunication.html
http://blog.csdn.net/anzhsoft/article/details/23428355
https://hadoop.apache.org/docs/r2.7.2/hadoop-project-dist/hadoop-hdfs/HDFSHighAvailabilityWithQJM.html
