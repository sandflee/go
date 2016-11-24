---
title: k8s apiserver分析
date: 2016-10-17
categories: k8s
tags: [k8s,apiserver]
---

# k8s apiserver

> The Kubernetes API server validates and configures data for the api objects which include pods, services, replicationcontrollers, and others. The API Server services REST operations and provides the frontend to the cluster’s shared state through which all other components interact.


## resource && Group && version
### resource
#### resource描述
pod service这类对象，etcd上存储的最小单位。
一个资源的描述一般包括４部分,
1. TypeMeta　资源的元信息，资源的类型，属于哪个Group/version
2. ObjectMeta　对象的元信息，对象的名字,label,annotation等
3. Spec  对象期望的状态
4. Status　对象实际的状态

```go
type Pod struct {
	unversioned.TypeMeta `json:",inline"`
	ObjectMeta `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`
	Spec PodSpec `json:"spec,omitempty" protobuf:"bytes,2,opt,name=spec"`
	Status PodStatus `json:"status,omitempty" protobuf:"bytes,3,opt,name=status"`
}
```



### Group 
一般类似功能的资源放到一个Group下,比如batch Group下面有job和ScheduledJob. 一些不成熟的会放到extensions

### version 
每一个Group都会有不同version（升级，向前兼容）, resource从属于一个version,version从属于Group,如果要升级到新的version,需要把钱一个version的resource在新version中实现，并创建转换函数负责不同版本间相同resource的convert
每个Group都必须提供unversioned的resource,供apiserver和其他模块使用。


## ApiServer分层

1. REST接口层，　对用户暴漏REST接口
2. resource storage层，具体某个资源的实现．每个REST接口都跟一个storage关联，storage提供了Create/Update/Delete/Get/Watch等接口。一般基于generic#store实现，每种resource只需要实线特定的stratgy,generic#store负责回调.用户只需要关心具体的实现策略即可
3. cache层　如果启用--watch-cache，会有额外的cache层(cacher.go)，如果没有启用,generic#store直接操作raw storage
4. raw storage层, 跟etcd打交道，有etcd集群信息和版本信息，把数据直接更新到etcd，

### REST接口
api_installer.go#registerResourceHandlers将resource和具体的RestApi绑定起来．
1. 创建decoder,decoder负责将version对象字节流decode成unversion对象
2. 必要时(CREATE/UPDATE/DELETE)进行准入控制
3. 调用resource storage相关接口（创建调用create,更新调用update等）

### resource storage层
#### storage向apiserver注册
每个ApiGroup需要创建ApiGroupInfo,里面包含group的版本，以及每个版本的resource map. ApiServer根据ApiGroup Info将其跟rest接口绑定
*master.go#installApis* 作为注册的总入口，创建对应ApiGroup并将其注册
##### 1. 创建ApiGroupInfo
groupMeta为每个Group install.go中注册的GroupMeta, Scheme,ParameterCodec,NegotiatedSerializer为全局变量，不需要额外创建
VersionedResourcesStorageMap　记录了每个GroupVersion都要哪些resource,以及对应的storage实现

```go
genericApiserver.go

APIGroupInfo {
   GroupMeta apimachinery.GroupMeta
   VersionedResourcesStorageMap [][]rest.Storage
   IsLegacyGroup bool
   OptionsExternalVersion *unversioned.GroupVersion

   Scheme *runtime.Scheme
   NegotiatedSerializer runtime.NegotiatedSerializer
   ParameterCodec runtime.ParameterCodec

   SubresourceGroupVersionKind []unversioned.GroupVersionKind
}
```
##### 2. 从ApiGroupInfo生成ApiGroupVersion，对应每个版本的信息
apiGroupInfo对应的是一个ApiGroup的信息，ApiGroupVersion对应其中一个特定版本
```go
func (s *GenericAPIServer) newAPIGroupVersion(apiGroupInfo *APIGroupInfo, groupVersion unversioned.GroupVersion) (*apiserver.APIGroupVersion, error) {
	return &apiserver.APIGroupVersion{
		RequestInfoResolver: s.NewRequestInfoResolver(),

		GroupVersion: groupVersion,

		ParameterCodec: apiGroupInfo.ParameterCodec,
		Serializer:     apiGroupInfo.NegotiatedSerializer,
		Creater:        apiGroupInfo.Scheme,
		Convertor:      apiGroupInfo.Scheme,
		Copier:         apiGroupInfo.Scheme,
		Typer:          apiGroupInfo.Scheme,
		SubresourceGroupVersionKind: apiGroupInfo.SubresourceGroupVersionKind,
		Linker: apiGroupInfo.GroupMeta.SelfLinker,
		Mapper: apiGroupInfo.GroupMeta.RESTMapper,

		Admit:             s.AdmissionControl,
		Context:           s.RequestContextMapper,
		MinRequestTimeout: s.MinRequestTimeout,
	}, nil
}
```

##### 3. ApiGroupVersion中的每个资源注册到apiserver
api_installer.go#registerResourceHandlers把下面storage中的resource和storage做绑定
```go
		petsetStorage, petsetStatusStorage := petsetetcd.NewREST(restOptionsGetter(apps.Resource("petsets")))
		storage["petsets"] = petsetStorage
```

#### generic storage实现
generic storerage在进行实际的etcd操作前进行了很多hook,用户只需要实线具体的stratery即可
其具体实现在*generic/registry/store.go*
##### create
1. 执行rest#BeforeCreate，
1.1 strategy.PrepareForCreate
1.2 创建uuid,如果没有名字产生名字
1.3 strategy.Validate验证资源的合法性
2. 底层storage执行create
3. 执行AfterCreate回调

##### update  todo
##### Delete  todo
##### Get todo

##### rest　策略层
提供了BeforeUpdate/BeforeCreate/BeforeDelete的实现，会回调每个资源的一些创建，更新策略
```go
type RESTCreateStrategy interface {
	runtime.ObjectTyper
	// The name generate is used when the standard GenerateName field is set.
	// The NameGenerator will be invoked prior to validation.
	api.NameGenerator

	// NamespaceScoped returns true if the object must be within a namespace.
	NamespaceScoped() bool
	// PrepareForCreate is invoked on create before validation to normalize
	// the object.  For example: remove fields that are not to be persisted,
	// sort order-insensitive list fields, etc.  This should not remove fields
	// whose presence would be considered a validation error.
	PrepareForCreate(ctx api.Context, obj runtime.Object)
	// Validate is invoked after default fields in the object have been filled in before
	// the object is persisted.  This method should not mutate the object.
	Validate(ctx api.Context, obj runtime.Object) field.ErrorList
	// Canonicalize is invoked after validation has succeeded but before the
	// object has been persisted.  This method may mutate the object.
	Canonicalize(obj runtime.Object)
}

type RESTGracefulDeleteStrategy interface {
	// CheckGracefulDelete should return true if the object can be gracefully deleted and set
	// any default values on the DeleteOptions.
	CheckGracefulDelete(ctx api.Context, obj runtime.Object, options *api.DeleteOptions) bool
}

type RESTUpdateStrategy interface {
	runtime.ObjectTyper
	// NamespaceScoped returns true if the object must be within a namespace.
	NamespaceScoped() bool
	// AllowCreateOnUpdate returns true if the object can be created by a PUT.
	AllowCreateOnUpdate() bool
	// PrepareForUpdate is invoked on update before validation to normalize
	// the object.  For example: remove fields that are not to be persisted,
	// sort order-insensitive list fields, etc.  This should not remove fields
	// whose presence would be considered a validation error.
	PrepareForUpdate(ctx api.Context, obj, old runtime.Object)
	// ValidateUpdate is invoked after default fields in the object have been
	// filled in before the object is persisted.  This method should not mutate
	// the object.
	ValidateUpdate(ctx api.Context, obj, old runtime.Object) field.ErrorList
	// Canonicalize is invoked after validation has succeeded but before the
	// object has been persisted.  This method may mutate the object.
	Canonicalize(obj runtime.Object)
	// AllowUnconditionalUpdate returns true if the object can be updated
	// unconditionally (irrespective of the latest resource version), when
	// there is no resource version specified in the object.
	AllowUnconditionalUpdate() bool
}
```

### cacher层
对watch请求进行cache,其他的Get/Update/Create/Delete直接走raw storage层．
#### 创建storage
master.go 创建generic.RESTOptions时，通过storageDecorator赋值给Decorator

```go
genericapiserver.go
func (s *GenericAPIServer) StorageDecorator() generic.StorageDecorator {
	if s.enableWatchCache {
		return registry.StorageWithCacher
	}
	return generic.UndecoratedStorage
}
```
```go
master.go
generic.RESTOptions{
		StorageConfig:           storageConfig,
		Decorator:               m.StorageDecorator(),
		DeleteCollectionWorkers: m.deleteCollectionWorkers,
		ResourcePrefix:          c.StorageFactory.ResourcePrefix(resource),
	}
```
``` go
type RESTOptions struct {
     // etcd相关配置，etcd2/etcd3? etcd location/prefix等，还包括codec, resource memory version和storageVersion相互转换
	StorageConfig           *storagebackend.Config
    // storage的修饰器，返回一个func，生成具体的storage接口，分为storageWithCacher和UndecodedStorage
	Decorator               StorageDecorator
	DeleteCollectionWorkers int

	ResourcePrefix string
}
```

- 每个Group都需要创建store对象，调用RestOptions.Decorator生成storage(cacher or raw)

```go
    registry#tapp/etcd#etcd.go
	storageInterface, _ := opts.Decorator(
		opts.StorageConfig,
		cachesize.GetWatchCacheSizeByResource(cachesize.TApp),
		&gaiaapi.TApp{},
		prefix,
		tapp.Strategy,
		newListFunc,
		storage.NoTriggerPublisher,
	)
    store := &registry.Store{
      Storage: storageInterface,
    }
```
#### cacher实现 todo
### raw storage层
etcd　lib的具体实现．etcd2的实线在storage/etcd_helper.go, etcd3的实线在storage／etcd3/store.go
Note:
1.具体存储前调用encoder将unversion resource转换成version　resource字节流
2.没有字段存储ResourceVersion，采用etcd modify index.


## what happend when create a resource？
1. 客户端通过RestApi请求创建petset
2. apiserver 执行回调函数restHandler#createHandler, 将字节流转换成unversion resource object,通过准入控制后，执行generic#store.Create
3. generic#store.create　流程参见前面描述，调用cacher#create
4. cacher#create不做处理直接调用raw storage回调，如果为etcd2执行etcd_helper#create
5. raw storage etcd helper将unversion object转换为versioned object并存储在etcd
6. raw storage 将etcd返回的value decode成unversion resource, 并根据返回的modifyIndex设置对象的resourceVersion
7. generic#store 执行AfterCreate回调
8. apiserver　将unversion resource转换为version resource并返回



###scheme 记录GroupVersionKind和type的映射关系
主要用于不同version resource的相互转换
重要接口：
1. addKnownTypes　
2. addDefaultFuncs
3. addConversionFuncs    
4. AddFieldLabelConversionFunc　　　field label?
