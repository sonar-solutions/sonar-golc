# Supported languages

To list all supported languages:

```bash
golc -languages
```

Example output:

| Language           | Extensions                               | Single Comments | Multi Line |
|--------------------|------------------------------------------|-----------------|------------|
| Abap               | .abap, .ab4, .flow, .asprog              | *, "            |            |
| ActionScript       | .as                                      | //              | /* */      |
| Apex               | .cls, .trigger                           | //              | /* */      |
| C                  | .c                                       | //              | /* */      |
| C Header           | .h                                       | //              | /* */      |
| C++                | .cpp, .cc                                | //              | /* */      |
| C++ Header         | .hh, .hpp                                | //              | /* */      |
| C#                 | .cs                                      | //              | /* */      |
| COBOL              | .cbl, .ccp, .cob, .cobol, .cpy           | *               |            |
| CSS                | .css                                     |                 | /* */      |
| Dart               | .dart                                    | //              | /* */      |
| Docker             | Dockerfile, dockerfile                   | #               |            |
| Golang             | .go                                      | //              | /* */      |
| HTML               | .html, .htm, .cshtml, .vbhtml, .aspx, …  |                 | <!-- -->   |
| Java               | .java, .jav                              | //              | /* */      |
| JavaScript         | .js, .jsx, .jsp, .jspf                   | //              | /* */      |
| Kotlin             | .kt, .kts                                | //              | /* */      |
| PHP                | .php, .php3, .php4, .php5, .phtml, .inc  | //, #           | /* */      |
| Python             | .py                                      | #               | """ """, ''' ''' |
| Ruby               | .rb                                      | #               | =begin =end |
| Rust               | .rs                                      | //              | /* */      |
| Scala              | .scala                                   | //              | /* */      |
| Shell              | .sh, .bash, .zsh, .ksh                   | #               |            |
| SQL                | .sql                                     | --              | /* */      |
| Swift              | .swift                                   | //              | /* */      |
| Terraform          | .tf                                      | #, //           | /* */      |
| TypeScript         | .ts, .tsx                                | //              | /* */      |
| Vue                | .vue                                     |                 | <!-- -->   |
| XML                | .xml, .XML                               |                 | <!-- -->   |
| YAML               | .yaml, .yml                              | #               |            |
| …                  | (and more)                               |                 |            |

❗️ To add a new language, add an entry to the Languages structure in [assets/languages.go](https://github.com/sonar-solutions/sonar-golc/blob/main/assets/languages.go) in the repository.
