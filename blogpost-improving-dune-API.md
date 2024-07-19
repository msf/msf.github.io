# A short story of how we've improved Dune API by using DuckDB

At Dune, we value our customers’ feedback and are committed to continuously improving our services. This is the story of how a simple, prioritized feature request—supporting query result pagination for larger results—evolved into a comprehensive improvement involving the adoption of DuckDB at Dune.

We’ve learned a lot during this journey and are excited to share our experiences and the new functionalities we’ve been building.

## Outline

- Motivation & Context
- Seeing further & expanding the use cases served by Dune API
- DuneSQL & Query Results
- Using DuckDB as a stepping stone
- final architecture
- New APIs launched

## Motivation & Context

The journey began with user feedback and a repeated feature request: “Dune API doesn’t support pagination, and the maximum size of query results is limited (~1GB).” Users needed to read larger results, which required supporting pagination. At the end of 2023, we finally prioritized resolving this issue. This feature request was the catalyst that sparked this work.

At the end of 2023 we FINALLY prioritized resolving this. NOTE: here's the [resulting Pagination API](https://docs.dune.com/api-reference/executions/pagination)


## Why no pagination and 1GB limit?

To address this question, let’s start with understanding our initial architecture and its limitations.

Our original capabilities were designed to serve the needs of [Dune Analytics](https://DUNE.COM), a platform focused on visualizing crypto data through dashboards and graphs. This use case leveraged the following architectural decisions:

  1. Query-Driven Visualizations: Each visualization on a dashboard was tied to a specific query ID. This setup allowed for consistent and static data views, which were suitable for our initial visualization-centric use cases.
  1. Powerful Query Execution: SQL Queries offer the expressiveness and capability for rich and complex data manipulations required to query and aggregate large datasets
  1. Small, Reusable Query Results: Visualizations typically require manageable data sizes, optimized for quick rendering on dashboards. Large datasets were unnecessary, as visual elements have limited pixel space and do not need millions of data points.
  1. Caching: To improve performance, we cached query results. This approach was suitable for dashboards that repeatedly accessed the same query results, reducing the need for re-execution.

However, these design choices led to significant limitations:

- 1GB Result Cap: The API was capped at 1GB per query result because larger results were unnecessary for visualization purposes and could overwhelm the system’s memory.
- No Pagination: Since visualizations generally required the entire dataset at once, there was no need for pagination.
- Expensive Query Execution: our query execution can be too slow or computationally expensive

## Expanding the Use Cases Served by Dune API

To truly serve our developer community, we needed to look holistically at their needs, beyond just supporting pagination. Our goal was to create features that seamlessly integrate with each other and existing Dune functionalities.

### Understanding Developer Needs Through User Stories

Instead of focusing narrowly on specific feature requests, we explored real-world use cases and “jobs to be done.” This approach helped us understand the broader context of developer needs. We crafted concrete user stories to guide our feature development:

  1. Search and Filter: Jesse needs an app to search and filter large query results by wallet address or time window, requiring advanced filtering and efficient handling of large datasets.
  2. Data for Charts: Logan needs to feed specific data into charts for mobile, requiring low network data usage and dynamic sorting and filtering.
  3. Data Science: Another use case involves performing data science on large sets of crypto transactions, emphasizing the need for pagination and efficient data handling.

**Fictitious Example: Joan’s Mobile App**

Consider Joan, a developer who wants to create a mobile app that visualizes Wallet Balances. Her app allows users to follow their wallet activities over time through infographics. For Joan’s app to be viable, it must be inexpensive to make multiple requests to DuneAPI for each user.

This example underscores the importance of thinking from the users’ perspective and visualizing the functionalities they need. It’s not just about a single feature but how different features can work together.

**Holistic Approach to API Development**

By stepping back and considering these diverse use cases, we aimed to build a more flexible API focused on application needs rather than strictly on SQL query execution. Our approach involved:

- Flexibility and Integration: Developing an API that easily integrates with existing Dune functionalities.
- Real-Life Applications: Using real-life applications, like Dune dashboarding, to guide feature development.
- Leveraging Technology: Using new technologies to bridge user needs and our engineering capabilities.
- Maximizing Value: Extending the use cases we cover with existing DUNE queries and query results, maximizing the value provided to our users.

This holistic approach not only addressed immediate feature requests but also paved the way for a more robust and versatile DuneAPI, empowering developers to build innovative applications.

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

## Final Architecture

So now at Dune we run & operate two database technologies that are directly used by our users: Trino & DuckDB, for both of them we have deeply integrated them and have specific APIs and features to better serve our users. We have also fully migrated all user queriable data: both the Tables and the Query Results to Parquet.

Our final architecture resembles this:
![Dune system's diagram for query execution](blogpost-query-systems-diagram.png)

## New Features and APIs powered by DuckDB

So, with this effort we have accomplished the basic goal of addressing important limitations on our [SQL Endpoints API](https://docs.dune.com/api-reference/executions/execution-object) by providing these functionalities:

- [Pagination](https://docs.dune.com/api-reference/executions/pagination): Retrieve data in manageable chunks to handle large datasets.
- [Filtering](https://docs.dune.com/api-reference/executions/filtering): Apply filters to query results based on specific columns and criteria.
relevance.
- [Sorting](https://docs.dune.com/api-reference/executions/sorting):  Organize query results in a specified order.
- [Sampling](https://docs.dune.com/api-reference/executions/sampling): Retrieve an uniform sample of the dataset for efficient data analysis and visualization.

But by integrating well with other functionalities of Dune (such as [Query Scheduler](https://docs.dune.com/web-app/query-editor/query-scheduler) and [Materialized views](https://docs.dune.com/query-engine/materialized-views) for example) it powers also:

- [Custom Endpoints](https://docs.dune.com/api-reference/custom/overview), which allow users to create and manage API endpoints from Dune queries.

- *Preset Endpoints*, which provide quick access to standardized and essential data, eliminating the need for custom queries for frequently needed information.

  - [Contracts](https://docs.dune.com/api-reference/evm/endpoint/contracts): Data on blockchain contracts.
  - [DEX Metrics](https://docs.dune.com/api-reference/dex/endpoint/dex_pair): Information on decentralized exchanges.
  - [EigenLayer](https://docs.dune.com/api-reference/eigenlayer/introduction) and [Farcaster](https://docs.dune.com/api-reference/farcaster/introduction): Metrics and data related to specific projects and technologies.
  - [Marketplace Marketshare](https://docs.dune.com/api-reference/markets/endpoint/marketplace_marketshare): Key market indicators and trends.
  - [Linea](https://docs.dune.com/api-reference/projects/endpoint/linea_lxp): Insights and data on specific blockchain project

## Conclusion

As we navigated the journey of enhancing DuneAPI, we experienced firsthand “feature creep”—you know, that moment when you start with a simple request and end up redesigning half the system. But feature creep doesn’t have to be scary. By stepping back and looking at the bigger picture of our customers’ needs, we found opportunities to innovate and build a better service.

Incorporating DuckDB was not just about addressing a single feature request; it was about rethinking how we can serve our users more effectively. By being open to evolving our approach and investing in scalable, efficient technologies, we’ve expanded the capabilities of DuneAPI, making it a powerful tool for developers.

We hope you find these new features and improvements valuable. As always, we’re excited to see how you’ll leverage them to create even more amazing applications and insights. Stay tuned for more updates, and keep those feedback and feature requests coming—they’re the real MVPs!

For more details, visit our API documentation.
