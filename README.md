# findme

`findme` is a powerful command-line utility designed to enhance file searching capabilities within directories on your system. It allows users to search for files containing specific text or patterns, with support for regular expressions for advanced query capabilities. Whether you're looking through your current directory or diving deep into nested folders, `findme` offers both precision and flexibility, making it an indispensable tool for developers, system administrators, and power users alike.

## Installation

Before installing `findme`, ensure you have Go installed on your system. `findme` can be installed directly via Go's package management tool.

```bash
go get -u github.com/genc-murat/findme
```

Or, clone the repository and build from source:

```bash
git clone https://github.com/genc-murat/findme.git
cd findme
go build
```

## Usage

To use `findme`, run it from the command line, specifying your search parameters. Below are some examples to get you started:

- **Basic Search**: Search for a query in all files within the current directory.

  ```bash
  findme search --dir "./" --query "your_search_query"
  ```

- **Regular Expression Search**: Use a regular expression for more complex searches.

  ```bash
  findme search --dir "./" --query "your_regex_pattern" --regex
  ```

- **Recursive Search**: Recursively search through directories and subdirectories.

  ```bash
  findme search --dir "./" --query "search_query" --recursive
  ```

## Features

- **Fast Searching**: Quickly find what you're looking for, even in large directories.
- **Regular Expression Support**: Leverage the power of regular expressions for complex searches.
- **Recursive Directory Search**: Easily search through all subdirectories.
- **Concurrent Processing**: Utilizes Go's concurrency for faster processing of large files.

## Contributing

Contributions to `findme` are welcome and greatly appreciated. If you're looking to contribute, please follow these steps:

1. Fork the repository.
2. Create your feature branch (`git checkout -b feature/AmazingFeature`).
3. Commit your changes (`git commit -m 'Add some AmazingFeature'`).
4. Push to the branch (`git push origin feature/AmazingFeature`).
5. Open a pull request.

Before submitting your contribution, please make sure to test your changes thoroughly.

## License

Distributed under the MIT License. See `LICENSE` for more information.

## Acknowledgments

- Special thanks to everyone who has contributed to making `findme` better.
