NewRelic BeeGo
==============

NewRelic BeeGo is "plug and play" package for monitoring (APM) BeeGo framework with NewRelic official agent<br />

Supports http endpoints
 
Can Get newrelic_beego.NewrelicAgent for custom monitoring(database,external call, func etc..)
Can Get newrelic transaction per request from beego context
```
txn := ctx.Input.GetData("newrelic_transaction").(newrelic.Transaction)
defer txn.EndDatastore(txn.StartSegment(), datastore.Segment{
    // Product is the datastore type.
    // See the constants in api/datastore/datastore.go.
    Product: datastore.MySQL,
    // Collection is the table or group.
    Collection: "my_table",
    // Operation is the relevant action, e.g. "SELECT" or "GET".
    Operation: "SELECT",
})
```

# Installation
```
dep ensure -v
```

Add  `_ "github.com/gtforge/newrelic_beego"` as an import in `main.go` file

# Available settings
    - appname = name of app in newrelic
    - newrelic_license = newrelic license
