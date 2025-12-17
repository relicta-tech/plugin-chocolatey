# Chocolatey Plugin for Relicta

Official Chocolatey plugin for [Relicta](https://github.com/relicta-tech/relicta) - Publish packages to Chocolatey (Windows).

## Installation

```bash
relicta plugin install chocolatey
relicta plugin enable chocolatey
```

## Configuration

Add to your `release.config.yaml`:

```yaml
plugins:
  - name: chocolatey
    enabled: true
    config:
      # Add configuration options here
```

## License

MIT License - see [LICENSE](LICENSE) for details.
