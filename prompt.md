We are building a dashboard for me to understand which files are up to date on Paperless.

I'd like to build something that uses the Paperless NGX api to connect to an instance of Paperless NGX and provide a quick dashboard about how up date various common documents should be. Examples of this might be things like:

* "Over the last 12 months, which months am I missing bills from Eversource"
* "Have I uploaded my tax receipts for this year, which normally are done in July?"
* "Do I have all of my bills uploaded from T-Mobile?"

These will probably configured using some sort of grammar - that indicates the correspondant, document type, possible tags, and timeframe to look.

We may also want to support additional/custom fields, as I have one called "amount" that I use frequently.

The goal is for me to get a single pain of glass that lets me easily see if I'm keeping my documents up to date inside of Paperless.

Additional features later on can include things like ensuring that the tags are consistent across all documents of a similar type (i.e. all of my Eversource bills should have the same tags), and showing trends in the custom metrics such as "amount", "kwh used", etc.
