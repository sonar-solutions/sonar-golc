# Reports

Report files are created in PDF, JSON, and CSV (for report-by-files).

```
Results
├── Byfile-report
│   ├── csv-report
│   │   └── Result……_byfile.csv
│   ├── pdf-report
│   │   └── Result……_byfile.pdf
│   └── Result……_byfile.json
└── Bylanguage-report
    ├── csv-report
    ├── pdf-report
    └── Result……_.json
├── GlobalReport.json
├── GlobalReport.pdf
├── GlobalReport.txt
```

To view results in a web interface, run the **ResultsAll** program. It will ask if you want to start the web UI, then start an HTTP service (default port 8091; you can choose another if 8091 is in use). Stop the server with **Ctrl+C**.

```bash
$ ./ResultsAll
```

From the web interface you can download report files in ZIP format.
