# controller manager
k8s用户只需要描述一个对象的desired state, 系统会根据desired state做一些操作，使得real state匹配desired state.
controller manager负责协调匹配各个资源的状态，其具体的逻辑通过功能独立的controller实现。

## controller介绍
### replication controller
作用：负责维护pod数目和rc期望的pod数目一致
实现：
监听apiserver中的rc和pod,当pod/rc发生变化，找到相应的rc(label匹配),做同步
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
维护短作业的生命周期
- 参数
job.Spec.Completions  pod完成几个后job认为已经成功
job.Spec.Parallelism  job的并行度，最多运行active的pod数目
- 如果设置了超时时间job.Spec.ActiveDeadlineSeconds，并且没有在这一段事件完成，会杀掉所有active pod,并把job状态设置为FAILED
- job controller监听job和pod对象，如果有相关变化，进行job同步
- job同步过程,  从podStore找到属于自己的pod, 并找出active,succ,fail的pod，如果succ pod数目大于job.Spec.Completions,认为job成功结束,如果小于,则对比期望的activePod数目和找到的activePod数目，如果不一致，创建/删除pod

### deployment-controller
新建／更新／删除／回退／deployment
labels[DefaultDeploymentUniqueLabelKey]　= hash(Spec.Template)
 = revision
- deployment把pod和replicaset一直发布。并且有一个操作版本的概念，可以对deployment升级，比如替换image,可以回退到某个版本。
- deployment controller负责监听deployment／replicaset/pod对象，如果发生变化则同步deployment
- deployment找出所有新的replicaset和老的replicaset，根据deployment.Spec.Strategy.Type，判断是几个几个升级还是把老的都kill掉（通过操作replicaset.spec.replica字段）
- 如何判断新老rs. hash(deployment.Spec.Template)得到一个value, 跟rs.labels[DefaultDeploymentUniqueLabelKey]比较，如果相同则是新的，如果不同，就是旧的
- 版本号的实现。rs.Annotations[deploymentutil.RevisionAnnotation]保存了当前rs的版本号，如果想回退到某个版本，只需要把这个版本的rs.spec.template　copy到 deployment.spec.Template，回退的版本就是最新的版本。
- 



### replicasets
> Replica Set is the next-generation Replication Controller. The only difference between a Replica Set and a Replication Controller right now is the selector support. Replica Set supports the new set-based selector requirements as described in the labels user guide whereas a Replication Controller only supports equality-based selector requirements.

###[Persistent-volume related](http://kubernetes.io/docs/user-guide/persistent-volumes/)
PersistentVolume (PV)作为一种资源被k8s管理，PersistentVolumeClaim (PVC)表示用户对PV资源的请求,使用的过程分为几个阶段
1. Provisioning, 用户创建pv
2. binding 用户创建pvc后，controller分配pv的过程，pv.spec.ClaimRef = pvc
3. using 用户pod使用pv
4. Releasing, 用户删除pvc
5. Reclaiming，　回收pv，涉及不同的回收策略

PV phase:
``` go
	// used for PersistentVolumes that are not available
	VolumePending PersistentVolumePhase = "Pending"
	// used for PersistentVolumes that are not yet bound
	// Available volumes are held by the binder and matched to PersistentVolumeClaims
	VolumeAvailable PersistentVolumePhase = "Available"
	// used for PersistentVolumes that are bound
	VolumeBound PersistentVolumePhase = "Bound"
	// used for PersistentVolumes where the bound PersistentVolumeClaim was deleted
	// released volumes must be recycled before becoming available again
	// this phase is used by the persistent volume claim binder to signal to another process to reclaim the resource
	VolumeReleased PersistentVolumePhase = "Released"
	// used for PersistentVolumes that failed to be correctly recycled or deleted after being released from a claim
	VolumeFailed PersistentVolumePhase = "Failed"
```
PVC phase:
```go
	// used for PersistentVolumeClaims that are not yet bound
	ClaimPending PersistentVolumeClaimPhase = "Pending"
	// used for PersistentVolumeClaims that are bound
	ClaimBound PersistentVolumeClaimPhase = "Bound"
```
#### persistent-volume-provisioner
- reconcileClaim 如果是新的claim,　调用plugin#NewProvisioner接口创建privisioner，最终创建persistemVollumn, 跟claim绑定

```go
if claim.annotations[pvProvisioningRequiredAnnotationKey] == pvProvisioningCompletedAnnotationValue
    return
provisioner = controller.newProvisioner()
newVollumn = provisioner.NewPersistentVolumeTemplate()
newVolume.Spec.ClaimRef = claimRef
newVolume.Annotations[pvProvisioningRequiredAnnotationKey] = "true"
controller.client.CreatePersistentVolume(newVolume)
claim.Annotations[pvProvisioningRequiredAnnotationKey] = pvProvisioningCompletedAnnotationValue
controller.client.UpdatePersistentVolumeClaim(claim)
```

- reconcileClaim 调用privisioner#Provision　分配具体的资源

```go
if pv.Spec.ClaimRef == nil || pv.annotations[pvProvisioningRequiredAnnotationKey] == pvProvisioningCompletedAnnotationValue 
   return
provisioner := controller.newProvisioner(controller.provisioner, claim, pv)
provisioner.Provision(pv)
pv.Annotations[pvProvisioningRequiredAnnotationKey] = pvProvisioningCompletedAnnotationValue
controller.client.UpdatePersistentVolume(volumeClone)    
```

#### persistent-volume-binder
- syncVolumn 等待volumn　provision完成，从pending状态到Available状态时会如果claim还是处于pending状态，会调用syncClaim，进行绑定
- syncclaim 等待claim provision完成。claim如果处于pending状态，会选择一个pv(acessMode符合，capacity浪费最小)并绑定，进入Bound状态。
```
volume = findBestMatchForClaim(claim)
claim.Spec.VolumeName = volume.Name
binderClient.UpdatePersistentVolumeClaim(claim)
claim.Status.Phase = api.ClaimBound
claim.Status.AccessModes = volume.Spec.AccessModes
claim.Status.Capacity = volume.Spec.Capacity
binderClient.UpdatePersistentVolumeClaimStatus(claim)
```

#### persistent-volume-recycler
如果persistentVolume处于released状态，根据Spec.PersistentVolumeReclaimPolicy回收资源
- PersistentVolumeReclaimRecycle, 调用插件的recycle函数，并且persistent-volume变为pending状态等待被绑定
```
volRecycler = plugin.NewRecycler(spec)
volRecycler.Recycle()
pv.Status.Phase = api.VolumePending
recycler.client.UpdatePersistentVolumeStatus(pv）
```
- PersistentVolumeReclaimDelete 调用插件的deleter删除pv,并向apiserver发送请求删除
```
deleter = plugin.NewDeleter(spec)
deleter.Delete()
recycler.client.DeletePersistentVolume(pv)
```



### tokens-controller
维护serviceAcount和secret的对应关系: 一个serviceAccount可能会对应多个secret,每个secret都有一个token.
- 监听serviceAccount, 如果增加/更新serviceAcount，如果没有secret跟serviceAccount绑定，则创建secret并绑定。如果删除serviceAccount,从apiserver删除相关的secret.
```
	secret := &api.Secret{
		ObjectMeta: api.ObjectMeta{
			Name:      secret.Strategy.GenerateName(fmt.Sprintf("%s-token-", serviceAccount.Name)),
			Namespace: serviceAccount.Namespace,
			Annotations: map[string]string{
				api.ServiceAccountNameKey: serviceAccount.Name,
				api.ServiceAccountUIDKey:  string(serviceAccount.UID),
			},
		},
		Type: api.SecretTypeServiceAccountToken,
		Data: map[string][]byte{},
	}
    token, err := e.token.GenerateToken(*serviceAccount, *secret)
    secret.Data[api.ServiceAccountTokenKey] = []byte(token)
	secret.Data[api.ServiceAccountNamespaceKey] = []byte(serviceAccount.Namespace)
    secret.Data[api.ServiceAccountRootCAKey] = e.rootCA
    e.client.Core().Secrets(serviceAccount.Namespace).Create(secret);
    liveServiceAccount.Secrets = append(liveServiceAccount.Secrets, api.ObjectReference{Name: secret.Name})
    serviceAccounts.Update(liveServiceAccount)
```
- 监听secret, 如果增加/更新secret,如果找不到相应的serviceAccount，删除secret.如果secret.token不存在会生成新的token,如果删除secret,会把secret从serviceAccount中删除，并更新serviceAccount

### service-account-controller
保证“default” serviceAccount的存在
- 监听"default"这个serviceAcount对象，如果被删除了，重新创建
- 监听namespace,如果新增/更新namespace，如果没有serviceAcount创建"default"serviceAccout



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
