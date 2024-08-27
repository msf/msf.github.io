# DuckCon & SpiceAI meetings.

This is a report of my attenance of DuckCon

DuckCon is the yearly conference for DuckDB https://duckdb.org/, an extremely high performance online analytics database that is very lightweight, can run in the browser (or on a traditional server).

In case you're not following the latest craze about DuckDB here are key highlights that make it super attrative:
1. Native support for querying Parquet, JSON, CSV, fantastic for data-wrangling.
1. Native S3, databricks, iceberg, and other data-lake systems. Amazing support for query federation.
1. It can run natively in Python and works natively with dataframes and you can mix and match SQL + Pandas/Numpy for amazing productivity.
1. It is in C++ and is like SQLite an embbeded DB that runs *inside* the application (beside python, you can run it on the browser or on the api server)
1. It is *extremely* user friendly and easy to run, there is no server required.
1. It really is very performant and very lightweight on resources.

My scope is the technical/engineering angle, enabling or breakthrough tech, spotting opportunities.

## DuckCon #5
https://duckdb.org/2024/08/15/duckcon5

There are 3 big highlights, all of them relevant to Dune Analytics.
1. BigData is DEAD -> this is the counter-movement to big large massively parallel query engines. (bigQuery, databricks, snowflake, etc..)
1. pg_duckdb: embbed DuckDB directly in your PostgreSQL, access directly both worlds, do your OLAP using duckdb, your OLTP on pgSQL.
1. duckdb on ETL pipelines -> Many reports of people replacing Trino or Spark with duckdb on specific ETL pipeline steps. Plenty of mentions of dbt+duckdb.
1. Mosaic: this is a dashboarding library/framework used in 3 different demos/presentations that had realtime dynamic dashboards
    (imagine the charts adjusting in realime as you zoom-in, pan, or apply a filter)

I'm going to leave "BigData is DEAD" for last, it is the more meaty subject and it shows up again on the SpiceAI meeting(link)


### PG_DUCKDB

pg_duckdb (https://github.com/duckdb/pg_duckdb) is a Postgres extension that embeds DuckDB's columnar-vectorized analytics engine and features into Postgres. We recommend using pg_duckdb to build high performance analytics and data-intensive applications.

TL;DR:
    This is a big deal, it is **very easy** way of having PgSQL with native support to interact with "big data systems" and also import/export data directly to s3://file.parquet



Longer summary:
This is an open source implementation of a postgres extention that embbeds a duckdb database, running in the same postgres server memory space.
The first direct implication is that you can have tables that are served by duckdb query engine and all the supported functionalities of duckdb and others that continue to be served by PgSQL engine.
Secondly, it now becomes suuuper convenient to run your internal analytics on (a read-replica of) your main postgres database, where you load pg_duckdb and use exclusively duckdb for analytics. You do this by creating matviews and exports of data to duckdb in the form of duckdb native tables and then make all analytics use those duckdb tables.
Thirdly, you can now very easily export/imports to/from S3+Parquet for Postgres (this is so smooth is almost like having native support for this on pgsql)
Forth, you can have queries that use tables in pgSQL + DuckDB without any problems, which is very convenient.
Finally, the major benefits I mentioned above can be used incrementally and piece meal, which is super amazing.

Big asterisk: this postgres extension isn't supported on RDS yet and there was lots of chatter about and how to make community pressure for AWS to allow the extension on RDS.

Direct implications:
1. Internal use of this tech: We could migrate our analytics DB to postgres+duckdb, it owuld be less work than migrating to clickhouse and I don't see any upside of picking clickhouse instead of postgres + pg_duckdb
1. Flywheel team datascience/recommendations features: I suspect that duckdb would be a great tool to use w/ our coreDB data, to allow for creating matviews and data-science data analytics we use for recommendations. It could replace typesense for denormalized large tables.


### DuckDB on ETL steps

This one is a short one: There are many people reporting using DuckDB on ETL pipelines. The reason is super simple:
1. Amazing support for CSV, JSON, Parquet
1. Many ETL steps are "consume Table" -> produce "parquet table"
1. it can connect to lots of data-lakes and database.
1. It is quite fast
1. you can use python and mix pandas + SQL
1. Also has good DBT support (https://github.com/duckdb/dbt-duckdb and https://docs.getdbt.com/docs/core/connect-data-platform/duckdb-setup)

So the outcome, is that people are replacing Trino or Spark with it, they get faster ETL execution, simpler code to maintain and end up saving money in the process.

We should check if we can run some of our spellbook stuff using DuckDB (or our new prices stuff?)


### Mosaic


Mosaic https://github.com/uwdata/mosaic and https://idl.uw.edu/mosaic/ : An Extensible Framework for Linking Databases and Interactive Views

https://idl.uw.edu/mosaic/what-is-mosaic/
Mosaic is a framework for linking data visualizations, tables, input widgets, and other data-driven components, while leveraging a database for scalable processing. With Mosaic, you can interactively visualize and explore millions and even billions of data points.

A key idea is that interface components – Mosaic clients – publish their data needs as queries that are managed by a central coordinator. The coordinator may further optimize queries before issuing them to a backing data source such as DuckDB.

There were 3 different talks that showed dynamic web apps with zero-latency re-charting of data based on mouse sub-selections or live editing of SQL.
The speed was: under 100millis, it felt completely instantenous. The type of data exploration that this enables is amazing.

My take:
This type of work marries amazingly well with our query-results because they're a single parquet file. We could run this on dune.com and directly expose the parquet file to the webclient. Of course it doesn't need to be query-results, this would work with any data we'd expose directly as parquet files.




### Big Data is DEAD

I don't need to keep "spreading the gospel" on this. So I'll be short. 

In essence, data ingestion keeps growing but the queried data has a very large recency bias and it is always a much smaller subset of all the data.
The exact queries that span +10TB are (studies document this, backed by real world data from the AWS redshift team, https://assets.amazon.science/24/3b/04b31ef64c83acf98fe3fdca9107/why-tpc-is-not-enough-an-analysis-of-the-amazon-redshift-fleet.pdf) are < 0.05% of the cases. 
With the (exponential) growth of HW capacity, this has outpaced the queriable dataset produced by humans. The result is a single instance machine, with a powerful vectorized query engine (such as duckdb), can efficentily run queries against these datasets and be faster than the large distributed systems that split storage from the compute clusters (like Trino and Databricks).

The key idea for this to be not only practical, but *the best approach* is simple:
1. Have a decent machine, with a bunch of NVMEs w/ 10TB of space or more.
1. Query Federation, support remote and local dataset.
1. Aggressively cache data locally.
1. Federate Query Engine with push-down predicates to a remote database that holds the cold data (like databricks or trino, etc)
1. On the tail case of queryies that require data from remote storage and it is larger, let the query run in the remote database.

End result:
1. We run 99% of the queries in a single machine, faster and very cheaply.
1. We only run 1% of the cases in the databricks/snowflake/bigquery cluster, keeping its costs and compute power bounded.

In many cases, not even query-federation is needed, you just create stand-alone recurring data-exports to parquet and let the "big box" query those remote parquet files.




### DuckCon #5 live notes
These are the raw notes I took during the conference
#### updates



Sneak peek 1.1.0

Spaces/Usecases
1. interactive analysis
1. pipeline component
1. "creative" architectures

v1.0.0 -> snow duck

support contracts -> how they make money

#### Extensions: they've been working on thiz.
- new parsers
- new filesystems
- new aggregations

community extensions:
`install <ext_name> FROM community`

going to have a stable:
- C API
- rust will be using the C API

#### featuresss
1. pg_duckdb -> use duckdb inside pgSQL.
SQL functions:
1. histogram (harrah)
1. bunch more of SQL functionalities.. 
1. performance work, neverending perf on CTEs, Joins, pushdowns..etc.
#### future
1. extension ecosystem
1. lakehouse formats
1. optimizer improvs
  1. partition/sorting awareness
  1. cardinality estimation
1. extensible parser

#### motherduck

Frances (Pery) Talk, from Motherduck

"multiplayer duckdb"
- shared-system single-instance of duckdb across your laptop and the "cloud"
- so controlplane, authn/z, then moving some queries between computes and zero-copy..
- okay, not single instance, you can spawn a connection to the "instance", with python or some app and use the same "install" (I guess sharing the state..)
- [dual-execution] they extend the parser/planner so that they have a coordinated distributed execution of the plan between the laptop and the cloud.


- uwdata.github.io/mosaic

we should use duckdb for our own analytics

#### Jann is a crazy dude

talk.onefact.org
it's about assymmetric access to information and arbitrage.
for example on medical data from nyc and real estate prices..


#### Quack Attack
It's about having duckdb natively integrated with Dart.

basically, native support for duckDB in "Flutter", because flutter is the main reason to use Dart.


#### Duck for your dashboard.

Robert, "notebooks" (observable corp), the "web page is the machine"
So, there's cells where you just write queries or javascript..

Basically, duckdb WASM, make super interactive, debuonce mouse, dynamic renders, add a new query on duckdb, a new render.

there is a common pattern with mosaic, of making interactive UIs that generate SQL by following the mouse and re-rendering the data w/ results from the query, the query must return in < 50ms.


#### pg_duckDB

Joe and JD, HYDRA, "duckdb powered postgres"

they wrote the pg_duckdb, this is basically the "new way" of doing.
dune.com version 1, using coreDB and having the query-results on coreDB.

#### then it's ME

#### RillData

"1 minute from data to dashboard"
basically, it's all built on ETL + duckdb or ETL


#### 2y of windowing improvements

just about internal DB-engine improvements for windowing..

#### DBVerse
composable database libraries built on duckdb.
1. dbMatrix
1. dbSpatial
1. dbSequence

#### Scalable pipelines w/ DuckDB

Argo + DuckDB

Argo workflows, k8s job orchestrator



The other learning is the aggressive leveraging of a few ideas:
1. Query Federation
1. big fast machines with local NVMEs, that have 100GB/sec of disk bandwith and Loads of ram.
1. Use "materialized views" + DuckDB on the local machine as a "magic performance boost"


In short, big data is dead, we should just run 1 big box, enable query federation and be smart about doing local materialization for the hot data on local NVME stores.

A single (bare metal) server holds 150TB of NVME and has 50Gbps (up to 200Gbps) uplink.
The real workloads are smaller than 150TB and the outlier cases can be handled by having "remote tables" and materialized views locally stored.
S3 is relegated to long tail + bootstrapping logic.
Many attendees using duckdb on a single big box (150TB of NVME fits in a single box)

