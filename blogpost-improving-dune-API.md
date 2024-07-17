# A short story of how we've improved Dune API by using DuckDB

At Dune we really like to listen to our customers, we wanted to improve our API and started to collect user feedback, complains and bug reports.
This is a story of how a simple, prioritized feature request: Support Query Result Pagination for larger results
Evolved into a much more comprehensive improvement that involved adopting DuckDB at Dune.

We've learned a lot during this short time and wanted to share some of our experience and what we've been building..


# Outline:
 - Motivation & Context
 - Seeing further & expanding the use cases served by Dune API
 - DuneSQL & Query Results
 - Using DuckDB as a stepping stone
 - final architecture
 - The variety of new APIs launched
 - Self deprecating joke:
     - What? Mega feature creep :D 

## Motivation & Context

Lets start with the user feedback and the repeated "feature request":
Dune API doesn't support pagination and is the maximum size of que query result is limited (~1GB)

Users wanted to be able to read larger results and that required supporting larger results and that implied the need for pagination.
At the end of 2023 we FINALLY prioritized resolving this. [NOTE: here's the current Paginated API and known Limits]

### Why ?

Up until very recently, our API was focussed solely on executing Dune queries and being able to read their results.

The limitations of our service were also related to the original use of DUNE.COM[TODO: LINK], that exposes crypto data in easy visualizations, tells stories using dashboards, graphs and the queries are there to support that.

So, this leads into a basic architectural structure:
- Dashboards are made of one or more visualizations.
- Visualizations use (and re-use) the result of a given query.
- Queries are long lived, they are configured to have visualizations

This implies:
- we should cache our query results because it is repeatedly used.
- the results in principle aren't very large, they're designed to support visualizations that have limited pixel space and therefore can't make use of millions of datapoints.
- When we need the data, we need all of it (with rare exceptions)
- the App works better with small query results because it loads them into memory to render visualizations, etc..

The first DuneAPI was simply an API service that exposed the internal machinery we have to support this rich DUNE.COM APP. As such, it suffered from limitations such as:
- the execution of a query doesn't support >1GB (the app doesnt need it, in fact large results kill the app because we)
- there was no pagination support (not needed)
- the queries must reference a query-ID
- query execution is computationally expensive and slow

## Seeing further & expanding the use cases served by Dune API

For API uses of crypto data, we realized we were not serving a lot of valid use cases and it was pretty much impossible to serve a very normal set of use cases using our API, it was too limited and too stuck to internal limitations.

So we started to write up "usecases" of the API that started from simple stories such like: 
    TODO: put here two simple user stories of two Devs and their apps/cases (Jesse and Logan)
    - one user story needs to search/filter a large query result by wallet address or zoom in a particular time-window.
    - one user story feeds specifically data to chart
    - one user story uses pagination, such as doing data science on a much larger set of crypto transactions
 
Our approach is to create a very concrete real world examples and dig into the "jobs to be done", their specific needs but also broader context.

One fictitious example:
  "Joan, a developer wants to write a mobile app that will have visualizations of Wallet Balances, where the user of her app follows their Wallets activity through time in an infographics way. As such, Joan mobile app on each phone will render a slightly different data (per wallet address for example) of the data. Also, in such types of use, it must be truly inexpensive for the developer to do a dozen or so request to the DuneAPI to serve each users of the app for this to economically viable."

This is just a single example, the point here is thinking from the users' perspective and visualize the functionalities such a customer would need or want.

In summary, there are a few different angles we wanted to explore:
- Use this as the start of a new API that is a lot more flexible, focussed on Application needs and not pinned to execution of SQL queries.
- use a real life application as an example of use-cases the API should serve well (the Dune dashboarding functionality)
- Leverage new technology to bridge our user's needs and our small engineering team.
- Focus on maximizing value by extending the usecases we can cover with existing DUNE queries and query-results.


## DuneSQL & Query Results

All our data available at Dune is queriable with DuneSQL, users use it to query our 1.8Millions tables and produce the amazing insights we can see in the public dashboards on dune.com. Due to the vast amount of tables and their sizes, DuneSQL is a distributed query engine (Trino) that uses massive amounts of parallel compute to query our data-lake. DunesSQL queries are extremely powerful but heavyweight and the query response is around a dozen seconds (or quite a few more)

[NOTE: We've extended and modified Trino for our needs before[LINK HERE], we could extend it further..]

Being able to run a filter or a query on an existing query result would mean running a DuneSQL Query on it, this imples:
- DuneSQL must support reading/querying the cached result (which it didn't at the time)
- Executing a Query on DuneSQL, requiring more (costly) compute capacity
- DuneSQL supporting low-latency query response time, filtering a result or paginating should take less than  100-200 milliseconds.
- Execution must be inexpensive, so that a single app can do dozens of requests per interactive user on it.

In short, we would have an uphill battle to shoehorn DuneSQL to serve well our new needs. It just isn't the right tech for the task at hand.


## Using DuckDB as a stepping stone

Okay, lets build (or leverage) new Tech!

The options we have are:
- implement our needs directly ourselves ontop of the data-format we use for our query-results (which was compressed JSON at the time)
- Load our query-results into some database that we then leverage it to provide the needed functionalities.

We also need to consider possible growths of functionality beyong the exact ones that we highlighted, some of them are very easy to anticipate:
- allow for min/max/mean or some other aggregation function on some column of the result
- allow re-ordering the result by any column
- allow for much larger results (increase limit from 1GB to 20B or 50GB for example)

Some functionalities are very easy to implement ourselves (we could've just implemented pagination, and closed the "API pagination" issue on our issue tracker).
But trying to support much larger results, fast search/filtering on rows or columns or sampling would be akin to re-implementing a simplistic query execution engine. We decided to explore query engines that would have an easy way to implement our needs:
- support our query result format
- query execution time under 100 milliseconds
- inexpensive/lighweight, so that the user can run dozens/hundreds every minute for free


We explored using DuckDB as an embeded fast query engine, but running on our server side.
The "crazy idea" was: if we can load our query results into duckDB fast, then we can serve all requests of the new API from this DB, where the query response time will be in single-milliseconds. Allowing us to serve all the user's queries quickly and with marginal cost.

DuckDB is a full-blown, high performance embedded analytics database and has an extensive set of modern day features such as:
- supports querying and loading data directly from JSON and Parquet
- has a high performance SQL query engine we can lean into it to support the features we need and have an easy path to increase our functionality in the future.

Our query results just so happen to be stored in compressed JSON, so we could query them w/ DuckDB, additionally, we could migrate to Parquet as a format and reap multiple benefits: much smaller files, much faster loading and querying times and would the format is also compatible with DuneSQL opening future options on what we can do with query results, our vast data-lake and DuneSQL.

[NOTE: for the sake of brevity I'm not including many design considerations and non-functional requiremts such as high availability, fault tolerance, scalability, security (auth, confidentiality & integrity)]


## final architecture

So now at Dune we run & operate two database technologies that are directly used by our users: Trino & DuckDB, for both of them we have deeply integrated them and have specific APIs and features to better serve our users. We have also fully migrated all user queriable data: both the Tables and the Query Results to Parquet.


Our final architecture resembles this:
```blabla

(Pretty Drawing)

- DUNE APP
- DUNE API

both connect to Query Execution Service
- QES runs DuckDB queries
- QES runs DuneSQL queries
- QES has Query Results

DuneSQL deployments

Dune Data Lake
```


## The variety of new APIs launched


Describe and link to our API docs all the recently built new feature

## A new, more flexible API
### Composable features

We're extenting the API functionality of what can do with existing query-results. This isn't a brand new product. This is an expansion of what we can do with existing query-results. We want the implementation time to be relatively short, address known limitations without a major reachitecture.


Another way to think about this is: We want to expand the value in a composable way, complement the existing functionalities and architecture and boosting its value. 
For example, in DUNE, to keep dashboards up to date, users can already manage query refresh schedules, we can increase the value of both:
- Dune dashboards
- query schedule functionality
- the Dune API

By building features that improve the utility of each case or enabling new uses by leveragin existing features.
  - "free queries" on DuneAPI.
    - Result Filtering, Sorting and Sampling
  - Preset APIs backed by Dune Queries

## Self deprecating joke:
##   - What? Mega feature creep :D 



## What do I want the readers to get out the blog post
- try to decide one primary goal
- focus on that one














