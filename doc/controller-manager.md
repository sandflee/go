# controller manager

## controller介绍
### replication controller
作用：负责维护pod数目和rc期望的pod数目一致
实现：
监听apiserver中的rc和pod,当pod/rc发生变化，找到相应的rc,做同步
同步过程：

- rc中期望的pod数目，如果跟podCache中的数目不同，调用apiserver接口增加／删除pod
- 调用apiserver接口把rc.status.replica字段更新为最新的podshu数目

### garbage-collector
每隔２０s，将系统中已经结束的pod（pod.status.phase not in (RUNNING,PENDING,UNKNOWN)）从apiserver删除

### node-controller
node ready -> 非ready
将node上所有pod的readyConditioin设置为false
如果持续很长时间处于非ready状态，将node上的pod清理交给其他routine清理,根据 pod.DeletionGracePeriodSeconds的设置又分为实时清理和延迟清理。
有一个routine定期扫面绑定在node上的pod (pod.spec.nodemame != ""),  如果对应的node在nodeCache中找不到了，删除这个pod

### service-controller
维护service和loadBlancher的对应关系

- service有变化，　创建/删除对应的lb
- node发生变化，　调用lb update接口更新hosts列表


### endpoint-controller
维护service和endPoint对象的映射关系

- 启动时获取所有endPoint对象，同步对应的service
- watch service, 如果有service发生变化，同步service
- 同步service罗辑：　
-- 如果serivce已经删除，删除对应的endpoint对象
-- 如果service增加／变化，获取所有相关pod信息，利用podId,生成endPoint对象，调用apiserver接口进行更新
- 例子
-- service : {"ports":[{"protocol":"TCP","port":8000,"targetPort":80}],"clusterIP\":\"10.0.0.72\"}
-- endpoint : {"addresses":[{"ip":"1.1.1.1"},{"ip":"1.1.1.2"}],"ports":[{"port":80,"protocol":"TCP"}]}
- endpoint对象的变化由kube-proxy捕获，维护对应的路由信息


### resourcequota-controller
- quota跟踪的是request资源，不是limit资源
apiserver在创建对象时检查是否超过quota，如果超过则拒绝请求。
> $ kube-apiserver --admission-control=ResourceQuota

- resourcequota-controller监听resourcequota,Pod,service,rc,PersistentVolumeClaim,Secret资源，
如果resourcequota发生变化，pod状态发生变化（变成succ／fail），其他资源被delete则会触发 resourcequota的sync
- 同步过程，通过quota#registry接口获取相关resource的资源汇总，跟quota.status.used做比较,如果不相同则更新apiserver中的quota.status.used
- scope,  创建resourcequota时可以制定scope,计算资源使用量时首先会判断是否属于这个scope,
如果pod没有显示的资源请求，isBestEffort(pod)为true

```go
	switch scope {
	case api.ResourceQuotaScopeTerminating:
		return isTerminating(pod)
	case api.ResourceQuotaScopeNotTerminating:
		return !isTerminating(pod)
	case api.ResourceQuotaScopeBestEffort:
		return isBestEffort(pod)
	case api.ResourceQuotaScopeNotBestEffort:
		return !isBestEffort(pod)
	}
```

### namespace-controller
namespace 创建后处于active状态，可以在namespace下创建各种资源
如果删除namespace, 处于terminating状态，Namespace.ObjecMeta.DeletionTimestamp被设置为当前时间，namespace controller发现这一事件，清理namespace下已知的资源，清理完成后将"kubernetes"从Namespace.Spec.Finalizers中删除
Namespace.Spec.Finalizers为空时，把namespace从etcd中删除，这个逻辑主要是保护用户在自己namespace创建自己的资源类型，等待所有资源被删除后才会删除namespace


### horizontal-pod-autoscaler
负责根据pod负载情况自动增加/删除 pod
- 每一个hpa对象创建时会跟一个rc/deployment绑定, 后续对pod进行增加删除的动作通过rc/deployment的scale接口进行
> kubectl autoscale rc foo --max=5 --cpu-percent=80
- 系统默认只支持根据cpu负载进行auto scale,用户也可以添加自定义的metric信息。通过HeapsterMetrics获取metric信息
- horizontal-pod-autoscaler监听系统中的hpa对象，如果发生变化,进入hirozonal#reconcileAutoscaler,获取实际使用cpu的负载，并targe负载做比较，决定要不scale，如果需要，则操作rc／deployment scale接口，并更新hpa状态
```java
usageRatio := float64(*currentUtilization) / float64(targetUtilization)
if math.Abs(1.0-usageRatio) > 0.1 {
	return int(math.Ceil(usageRatio * float64(currentReplicas))), currentUtilization, timestamp, nil
} else {
	return currentReplicas, currentUtilization, timestamp, nil
}
```
- 如果状态更新马上触发watch操作？　什么时候触发下一次reconcile?

### daemon-set-controller
控制在node上启动指定的pod,如果指定了.spec.template.spec.nodeSelector或.spec.template.metadata.annotations，会在匹配的node上启动pod，否则在所有node上启动。daemonSet创建的pod直接指定pod.spec.nodeName不经过调度器调度

- 监听deemonSet, node, pod 三种resource, 下面集中情况会进行daemonSet同步
-- Add/update/Delete deamonSet
-- 如果有变化的pod相关的demaonSet(label匹配)，会把相关的DeamonSet进行同步
-- nodeAdd,nodeShouldRunDaemonPod返回除，nodeUpdate　nodeShouldRunDaemonPod(oldNode) !=　nodeShouldRunDaemonPod(NewNode)

- DaemonSet同步过程
-- 遍历podStore中deamonSet的所有pod,以nodeName为key放到map里
-- 遍历nodeStore中的node, 用nodeShouldRunDaemonPod判断是否可以运行pod，跟上面得到的结果做对比，判断是否需要增加/删除pod, 如果创建pod, pod.spec.nodeName指定为所在的nodeName,也就是创建的pod不需要经过调度器调度
- nodeShouldRunDaemonPod 会参考nodeCondition, 是否有空闲资源，是否pod端口冲突

### job-controller

### deployment-controller

### replicasets

### persistent-volume-binder

### persistent-volume-recycler

### persistent-volume-provisioner

### tokens-controller

### service-account-controller

## 数据结构
### informer
informer提供了当apiserver中的资源发生变化时，获得通知的框架
- 需要用户提供listWatcher从apiserver同步resource, 以及ResourceHandler接口当资源发生改变时回调Added/Deleted/updated接口。每个controller只需要完成ResourceHandler逻辑即可。
- 创建informer时，会创建一个store和controller，store保存了最新的resource在本地的cache, controller则通过listWatcher获取资源的最新信息，更新store,如果resource发生变化，回调ResourceHandler

### workQueue 
特殊的FIFO，如果在pop前，push一个对象多次，只能取出一个。informer判断对象需要同步时会把对象放入workQueue, worker负责具体的同步逻辑，因为是同步操作，所以只需要同步一次。

### DeltaQueue
- 类似FIFO队列，取出一个对象时，会把这段时间关于这个对象的所有操作取出来
```
DeltaQueue.add(a)
DeltaQueue.add(b)
DeltaQueue.add(b)
DeltaQueue.delete(a)
item, delta = DeltaQueue.get()
item == a
delta == [ADD,DETELE]
```
- replace方法，


### store
- 提供基本对象存储功能，有add/get/delete接口，底层实现依赖ThreadSafeStore
```go
// cache responsibilities are limited to:
//	1. Computing keys for objects via keyFunc
//  2. Invoking methods of a ThreadSafeStorage interface
type cache struct {
	// cacheStorage bears the burden of thread safety for the cache
	cacheStorage ThreadSafeStore
	// keyFunc is used to make the key for objects stored in and retrieved from items, and
	// should be deterministic.
	keyFunc KeyFunc
}
```
- threadSafeStore 基本可以认为是线程安全的map, 其中的indexers 感觉没什么用