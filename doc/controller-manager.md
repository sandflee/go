---
title: k8s controller manager分析
date: 2016-09-26
categories: k8s
tags: [k8s,controller]
---

# controller manager
k8s用户只需要描述一个对象的desired state, 系统会根据desired state做一些操作，使得real state匹配desired state.
controller manager负责协调匹配各个资源的状态，其具体的逻辑通过功能独立的controller实现。

PRE NOTE
2. 介绍中的watch一个资源，在具体实现的时候可能为listAndWatch
3. 一个资源的变化时需要找到相关的另一个资源，采用label匹配的方法
4. 更新/删除一个资源，一般表示通过apiserver接口更新／删除资源，最终回写到etcd
1. 进行资源同步时，所有的操作都在同一个namespace下


## controller介绍
### replication controller
作用：负责维护系统中alive的pod数目和rc期望的pod数目（rc.spec.replicas）一致
实现：
监听apiserver中的rc和pod,当pod/rc发生变化，找到相应的rc(label匹配),做同步
同步过程：

- rc中期望的pod数目N1(rc.spec.replicas)，如果跟podCache中alive的pod数目N2不同(pod.status.phase not in (FAILED,SUCCESSED) and pod.DeletionTimestamp != null)，调用apiserver接口增加／删除pod
- 调用apiserver接口把rc.status.replica字段更新为N2,
- 删除／新增 pod又会触发新一轮的同步，最终N1 ＝＝ N2

gaia目前通过container complete消息通知AM，AM根据失败类型重新申请container，拉起。其中一个环节出错，container的拉起会有问题。相应的要做很多容错处理（container complete通知机制，AM状态保存）
NOTE：pod的存在可以独立于rc

### node-controller
维护node的状态，
- 监听pod/node/deamonSet对象。pod对象，对pod.DeletionTimestamp > 0对pod,如果node不存在删除pod。监听node/deamonSet对象，缓存在本地cache中
- 周期性monitorNodeStatus
1. node被删除，清理上面的container
2. controller会记录最新的node READY condition, 如果当前的READY conditioin != saved condition,保存最新的ready condition并更新nodeStatus.probetimestamp, 如果probetimestamp很长时间没更新（默认40s），则认为node可能出现问题，将node READY condition变成UNKNOWN。并回写apiserver
3. node ready -> 非ready 将node上所有pod的readyConditioin设置为false
4. 如果node ready condition 为false/unknown 超过5min，清理上面的container
5. node 非ready变为ready，node controller没有动作，scheduler对这个感兴趣
6. 如果node处于非ready状态会向cloud请求node是否存在，如果不存在，把node从etcd中删除
NOTE: 清理pod时，如果pod属于DeamonSet,node controller不会清理，等待DaemonSet controller清理。


- 有一个routine定期扫面绑定在node上的pod (pod.spec.nodemame != ""),  如果对应的node在nodeCache中找不到了，删除这个pod

### petset

- 系统中petset的pod为Pod1, 期望的pod为Pod2, 需要同步的pod为pod2,需要删除的pod为pod1 - pod2
- 同步过程：　如果系统中没有ｐｏｄ，创建。如果有，则比较petId（对名字/网络／pvc identifier的签名）是否相同，如果不同则更新对应的pod
- 删除过程：　调用apiserver接口删除，只是更新deleteTimestamp,等待kubelet物理删除
- Note:
每个petset只能同时创建/删除一个pod.
创建一个pod后，需要等待pod状态变为running，才进行下一个操作。
删除pod后，需要等待pod从apiserver物理删除才进行下一个操作。
每个petset正在操作的pod会放入unhealthyPetTracker#store中

### service-controller
维护service和loadBlancer的对应关系

- service有变化，　创建/删除对应的lb
- node发生变化，　调用lb update接口更新hosts列表
- service有三类，clusterIP/NodePort/lb, 前两个资源的分配放到了apiserver


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
- endpoint对象的变化由kube-proxy捕获，维护对应的路由信息,其他pod就可以通过serviceIP访问service


### namespace-controller
- namespace 创建后处于active状态，可以在namespace下创建各种资源
- 如果删除namespace, 处于terminating状态，Namespace.ObjecMeta.DeletionTimestamp被设置为当前时间，namespace controller发现这一事件，清理namespace下已知的资源，清理完成后将"kubernetes"从Namespace.Spec.Finalizers中删除
- Namespace.Spec.Finalizers为空时，把namespace从etcd中删除，这个逻辑主要是保护用户在自己namespace创建自己的资源类型，等待所有资源被删除后才会删除namespace


### resourcequota-controller
- quota在一个namespace内限制，quota跟踪的是request资源，不是limit资源
apiserver在创建对象时检查是否超过quota，如果超过则拒绝请求。
> $ kube-apiserver --admission-control=ResourceQuota

- resourcequota-controller监听resourcequota,Pod,service,rc,PersistentVolumeClaim,Secret资源，
如果resourcequota发生变化，pod状态发生变化（变成succ／fail），其他资源被delete则会触发 resourcequota的sync。pod会影响内存／cpu quota,其他资源影响resource 数目quota
- 同步过程，通过quota#registry接口获取相关resource的资源汇总，跟quota.status.used做比较,如果不相同则更新apiserver中的quota.status.used
- scope,  创建resourcequota时可以制定scope,计算资源使用量时首先会判断pod是否属于这个scope,
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


### garbage-collector
每隔２０s，如果结束的pod（pod.status.phase not in (RUNNING,PENDING,UNKNOWN)）超过一定数目（默认12500），选出最老的pod从apiserver删除.


### horizontal-pod-autoscaler
负责根据pod负载情况自动增加/删除 pod
- 每一个hpa对象创建时会跟一个rc/deployment绑定, 后续对pod进行增加删除的动作通过rc/deployment的scale接口进行
> kubectl autoscale rc foo --max=5 --cpu-percent=80
- 系统默认只支持根据cpu负载进行auto scale,用户也可以添加自定义的metric信息。通过HeapsterMetrics获取metric信息
- 周期性进行hpa对象同步，同步过程：hirozonal#reconcileAutoscaler,获取实际使用cpu的负载，并targe负载做比较，决定要不scale，如果需要，则操作rc／deployment scale接口，并更新hpa状态
```java
usageRatio := float64(*currentUtilization) / float64(targetUtilization)
if math.Abs(1.0-usageRatio) > 0.1 {
	return int(math.Ceil(usageRatio * float64(currentReplicas))), currentUtilization, timestamp, nil
} else {
	return currentReplicas, currentUtilization, timestamp, nil
}
```


### daemon-set-controller
控制在node上启动指定的pod,
如果指定了.spec.template.spec.nodeSelector或.spec.template.metadata.annotations，会在匹配的node上启动pod，否则在所有node上启动。daemonSet创建的pod直接指定pod.spec.nodeName不经过调度器调度

- 监听deemonSet, node, pod 三种resource, 下面集中情况会进行daemonSet同步
-- Add/update/Delete deamonSet
-- 如果有变化的pod相关的demaonSet(label匹配)，会把相关的DeamonSet进行同步
-- nodeAdd,nodeShouldRunDaemonPod返回除，nodeUpdate　nodeShouldRunDaemonPod(oldNode) !=　nodeShouldRunDaemonPod(NewNode)

- DaemonSet同步过程
１． 遍历podStore中deamonSet的所有pod,以nodeName为key放到map里
２．　遍历nodeStore中的node, 用nodeShouldRunDaemonPod判断是否可以运行pod，跟上面得到的结果做对比，判断是否需要增加/删除pod, 如果创建pod, pod.spec.nodeName指定为所在的nodeName,也就是创建的pod不需要经过调度器调度
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
deployment会把pod和rs一块儿发布。支持新建／更新／删除／回退／deployment

- deployment把pod和replicaset一直发布。并且有一个操作版本的概念，可以对deployment升级，比如替换image,可以回退到某个版本。
- deployment controller负责监听deployment／replicaset/pod对象，如果发生变化则同步deployment
- deployment找出所有新的replicaset和老的replicaset，根据deployment.Spec.Strategy.Type，判断是几个几个升级还是把老的都kill掉（通过操作replicaset.spec.replica字段）
- 如何判断新老rs. hash(deployment.Spec.Template)得到一个value, 跟rs.labels[DefaultDeploymentUniqueLabelKey]比较，如果相同则是新的，如果不同，就是旧的
- 版本号的实现。rs.Annotations[deploymentutil.RevisionAnnotation]保存了当前rs的版本号，如果想回退到某个版本，只需要把这个版本的rs.spec.template　copy到 deployment.spec.Template，回退的版本就是最新的版本。
- 升级的具体过程
- deployment 如何创建rs?  deployment的label对rs和pod都没有影响，annotation会传给rs.  hash key会传给template.label,最终影响rs和pod
1. newTemplate =  deployment.spec.template
2. add hashKey label to newTemplate.ObjectMeta.Labels (第一步已经把template中的label　copy过去)
3. newRS.spec.selector = deployment.Selector + hashKey selector
4. newRS.annotation = deployment.anotation
5. create rs object     
6. rs的label如何生成？　从结果上看是从template.labels上生成的

deployment_controller.go#getNewReplicaSet
```go
	newRS := extensions.ReplicaSet{
		ObjectMeta: api.ObjectMeta{
			// Make the name deterministic, to ensure idempotence
			Name:      deployment.Name + "-" + fmt.Sprintf("%d", podTemplateSpecHash),
			Namespace: namespace,
		},
		Spec: extensions.ReplicaSetSpec{
			Replicas: 0,
			Selector: newRSSelector,
			Template: newRSTemplate,
		},
	}
```

- rs如何创建pod?   
1. desiredLabels = template.labels
2. desiredAnnotations = template.annotations + createBy annotation
3. pod.spec = template.spec

```go
	pod := &api.Pod{
		ObjectMeta: api.ObjectMeta{
			Labels:       desiredLabels,
			Annotations:  desiredAnnotations,
			GenerateName: prefix,
		},
	}
```
- hash生成, 把hashKey从template.spec.labels中去除，对template做签名
``` go
func GetPodTemplateSpecHash(rs extensions.ReplicaSet) string {
	meta := rs.Spec.Template.ObjectMeta
	meta.Labels = labelsutil.CloneAndRemoveLabel(meta.Labels, extensions.DefaultDeploymentUniqueLabelKey)
	return fmt.Sprintf("%d", podutil.GetPodTemplateSpecHash(api.PodTemplateSpec{
		ObjectMeta: meta,
		Spec:       rs.Spec.Template.Spec,
	}))
}
```
- deployment　利用spec.template.metadata.labels生成selector，具体实现在kubectl/run.go#Generate


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

### service-account-controller
保证“default” serviceAccount的存在
- service Acout
```go
type ServiceAccount struct {
    TypeMeta   `json:",inline" yaml:",inline"`
    ObjectMeta `json:"metadata,omitempty" yaml:"metadata,omitempty"`

    username string
    securityContext ObjectReference // (reference to a securityContext object)
    secrets []ObjectReference // (references to secret objects
}
```
- 监听"default"这个serviceAcount对象，如果被删除了，重新创建
- 监听namespace,如果新增/更新namespace，如果没有serviceAcount创建"default"serviceAccout

### tokens-controller
维护serviceAcount和secret的对应关系: 一个serviceAccount可能会对应多个secret,每个secret都有一个token.
- 监听serviceAccount, 如果增加/更新serviceAcount，如果没有secret跟serviceAccount绑定，则创建secret,token并绑定。如果删除serviceAccount,从apiserver删除相关的secret.
创建secret过程：
```go
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





## 数据结构
### informer
informer提供了当apiserver中的资源发生变化时，获得通知的框架
- 需要用户提供listWatcher从apiserver同步resource, 以及ResourceHandler接口当资源发生改变时回调Added/Deleted/updated接口。每个controller只需要完成ResourceHandler逻辑即可。
- 创建informer时，会创建一个store和controller，store保存了最新的resource在本地的cache, controller则通过listWatcher获取资源的最新信息，更新store,如果resource发生变化，回调ResourceHandler
- indexInformer  创建store时用户可以传入indexer,做二级索引
- sharedIndexInforme

### reflector
- 利用listWatcher获取数据的变化
  1. client.list获取所有对象，调用deltaQueue#replcase方法（delete老数据,add新数据）
  2. 如果设置rsync period,每隔一段时间对deltaQueue中所有known key发送sync事件
  3. 不断watch新的事件，并将事件放入deltaQueue
- 将事件放入delta　queue
- process函数获取（pop）变化的事件，根据事件类型更新store,并触发注册的回调函数

### workQueue 

- 特殊的FIFO，如果在pop前，push一个对象多次，只能取出一个。informer判断对象需要同步时会把对象放入workQueue, worker负责具体的同步逻辑，因为是同步操作，所以只需要同步一次。
- 一个对象在同步时会被放入dirty　map中，保证同时只能被一个worker处理

### DeltaQueue
- 类似FIFO队列，取出一个对象时，会把这段时间关于这个对象的所有操作取出来
```go
DeltaQueue.add(a)
DeltaQueue.add(b)
DeltaQueue.add(b)
DeltaQueue.delete(a)
item, delta = DeltaQueue.get()
item == a
delta == [ADD,DETELE]
```
- replace方法，
- hasSynced
replace产生的对象已经都被poｐ完，　对应的store是一份完整的视图　（实现有bug?  delete的元素没有考虑进去）


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
- threadSafeStore 基本可以认为是线程安全的map, 其中的indexers提供了辅助索引的功能，实际系统中好像没什么用

### generation && observedGeneration
对象创建时generation为１，一般spec发成更改时，generation++, 以rs为例，实现在registry/replicaset/strategy.go#PrepareForCreate/PrepareForUpdate
status.ObservedGeneration, 以rs为例,syncRS时会把observedGeneration变成generation.
deployment更新rs spec后会等待generation == status.observedGeneration,才会进行下一步的动作，起到两个资源的同步作用。当他们相等时，说明对spec的改变rs已经recives
