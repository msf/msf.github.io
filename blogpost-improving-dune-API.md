# A short story of how we've improved Dune API by using DuckDB

At Dune we really like to listen to our customers, we wanted to improve our API and started to collect user feedback, complains and bug reports.
This is a story of how a simple, prioritized feature request: Support Query Result Pagination for larger results
Evolved into a much more comprehensive improvement that involved adopting DuckDB at Dune.
The end result is a much broader range of usecases that Dune can now serve, by combining a few complementary features.

We've learned a lot during this short time and wanted to share some of our experience and what we've been building..


Outline:
 - Motivation & Context
 - Seeing further & expanding the use cases served by Dune API
 - DuneSQL & Query Results
 - Using DuckDB as a stepping stone
 - Hackathon & real concrete use cases
 - final architecture
 - The variety of new APIs launched
 - Self deprecating joke:
     - What? Mega feature creep :D 



## TL;DR:
 - small feature requests made us think about expanding usecases and functionalities
 - the process & using new DB tech: duckDB & parquet
 - How using new tech enables a large set of cool features on "old things"
  - "free queries" on DuneAPI.
    - Result Filtering, Sorting and Sampling
 - Increasing user value by creating composable functionalities:
  - Query Schedules
  - API Endpoints
  - 
  - how this new functionality increases also the value of query schedules
  - and H
  - We can combine queries & query-schedules with the new API to offer complete new features like:
    - API Endpoints

## What do I want the readers to get out the blog post
- try to decide one primary goal
- focus on that one


# Motivation

Up until very recently, our API was focussed on executing dune queries and being able to read their results.

The limitations of our service were also related to the original use of our Dune.com portal, that exposes crypto data in easy visualizations, tells stories using dashboards, graphs and the queries are there to support that.

So, this leads into a basic architectural structure:
- Queries are long lived, they are configured to have visualizations
- Visualizations use (and re-use) the result of a given query.
- Dashboards are made of one or more visualzations.

This implies:
- we should cache our query results because the data is repeatedly used.
- the results in principle aren't super large, they're designed to support visualizations that have limited pixel space and therefore can't make use of millions of datapoints.
- When we need the data, we need all of it (with rare exceptions)
- the App wants small query results because it loads them into memory to render visualizations, etc..

The first DuneAPI was simply an API service that exposed the internal machinery we have to support this rich dashboarding APP. as such, it suffered from limitations such as:
- there was no pagination support
- the queries must reference a query-ID
- the execution of a query doesn't support >1GB (the app doesnt need it, in fact large results kill the app because we)
- query execution is computationally expensive and slow


# Ideas

For API uses of crypto data, we realized we were not serving a lot of valid use cases and it was pretty much impossible to build applications ontop of our API, it was too limited and too stuck to internal limitations.

So we started to write up "usecases" of the API that started from simple stories such like: 
    TODO: put here two simple user stories of two Devs and their apps/cases (Jesse and Logan)
    - one of the user stories focusses specifically on dealing with a large table with millions of rows where we want to search for wallet addresses or specific subranges of the resul
    - one user story feeds specifically data to chart
    - one user story uses pagination?
 
  "Joan, a developer wants to write a mobile app that will have visualizations of Wallet Balances, where the user of her app follows their Wallets activity through time in an infographics way. As such, Joan mobile app needs to , as usual on a mobile app, each phone will render a slightly different data (per wallet address for example) of the data. Also, in such types of use, it must be truly inexpensive for the developer to do a dozen or so request to the DuneAPI to serve each users of the app for this to economically viable.

This is just a single example, the point here is thinking from the users' perspective and visualize the functionalities such a customer would need or want.

## A new, more flexible API

There are a few different angles we wanted to explore:
- Use this as the start of a new API that is a lot more flexible, focussed on Application needs and not pinned to execution of SQL queries.
- use a real life application as an example of use-cases the API should serve well (the Dune dashboarding functionality)
- Leverage new technology to bridge our user's needs and our small engineering team.
- Focus on maximizing value by extending the usecases we can cover with existing dune queries and query-results.

## Technical Limitations

We're extenting the API functionality of what can do with existing query-results. This isn't a brand new product. This is an expansion of what we can do with existing query-results. We want the implementation time to be relatively short, address known limitations without a major reachitecture.

As such, it must work with the current data format of our query-results, which are stored as JSON and provided to the App or on the API as JSON.

### Can we use DuneSQL for these functionalities?

DuneSQL is our query engine, it is based on Trino. It is a fantastic, high performance, massively parallel distributed query engine


## Why not use DuneSQL for these functionalities? 
All our data available at Dune is queriable with DuneSQL, users use it to query our 1.8Millions tables and produce the amazing insights we can see in the public dashboards on dune.com. Due to the vast amount of tables and their sizes, DuneSQL is a distributed query engine (Trino) that uses massive amounts of parallel compute to query our data-lake. DunesSQL queries are extremely powerful but heavyweight and the latency profile it has doesn't work for the usecases we want to improve.
In essense, our needs are different from the strenghts that DuneSQL provides, we need a different tech-stack.

The options we have are:
- implement our needs directly ourselves ontop of the data-format we use for our query-results (which was JSON at the time)
- Load our query-results into some database that we then leverage to provide functionalities.

We also need to consider possible growths of functionality beyong the exact ones that we highlighted, some of them are very easy to anticipate:
- allow for min/max/mean or some other aggregation function on some column of the result
- allow re-ordering the result by any column
- allow for much larger results (which were at the time limited to 1GB)

# Hackathon

We do hackathons every 6months that last for 1 or 2 days and we're free to try out crazy ideas that would delight our users.
So, one of the ideas we want to try was to really create a great "dashboard" or "charting" experience on mobile. We started with a prototype API that would allow charting any result, irrespective of how large the query result is.
 - for a given query result, return only a subset of all the columns and sample the rows to just enough for a high-resolution time-series chart rendered on a 4k-8k screen. The crux was to force the server-side API to solve the problem of how to chart 1.2 Million rows in a lightweight way.

We explored using DuckDB as an embeded fast query engine, but running on our server side.
The "crazy idea" was: if we can load our query results into duckDB fast, then we can serve all requests of the new API from this DB, where the query response time will be in single-milliseconds. Allowing us to serve all the user's queries quickly and with marginal cost.

DuckDB is a full-blown, high performance embedded analytics database and has an extensive set of modern day features such as:
- supports querying and loading data directly from JSON and Parquet
- having a high performance SQL query engine we can lean into it to support the features we need and have an easy path to increase our functionality in the future.



# Rolling out to Production

# Other paths to follow

Preset APIs backed by Dune Queries

# Stabilization
