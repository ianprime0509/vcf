# vcf

Vcf is a program for searching and formatting vCard data. For full details on
how to use it, see the accompanying man page (`vcf.1`). To render the man page
on a typical GNU/Linux system, run `nroff -mdoc vcf.1 | less`; on BSD systems,
try `mandoc vcf.1 | less`.

For a taste of the features available, here are a few simple examples. Suppose
that the following text is saved in a file called `contacts.vcf`:

```
BEGIN:VCARD
VERSION:3.0
FN:John Smith
EMAIL;TYPE=HOME:jsmith@email.example
EMAIL;TYPE=WORK:john.smith@company.example
BDAY:19960509
END:VCARD
BEGIN:VCARD
VERSION:3.0
FN:Jane Smith
EMAIL;TYPE=HOME:js123@email.example
TEL:(555)555-5555
END:VCARD
```

Vcf allows you to search and format this information in a flexible way. The
simplest usage is to use the default options and get email listings for people
with a certain name:

```sh
$ vcf -i contacts.vcf john
John Smith <jsmith@email.example>
John Smith <john.smith@company.example>
```

To customize the output, provide a format string (analogous to how `date(1)`
works):

```sh
$ vcf -i contacts.vcf -f 'Name: %n\nEmail: %e'
Name: John Smith
Email: jsmith@email.example
Name: John Smith
Email: john.smith@company.example
```

See the man page for more advanced usage. Vcf is meant to integrate well with
other tools, and not to provide functionality that isn't necessary or that
would make the program too complicated.
