<?php

declare(strict_types=1);

namespace Flowmark\Board;

/** Reads the URL of the client entry that the asset build writes to its manifest (HY-76). */
final class Assets
{
    /** @param array{hyper: string} $urls */
    private function __construct(private readonly array $urls)
    {
    }

    /** Reads the manifest; a missing manifest means the assets were not built. */
    public static function fromManifest(string $file): self
    {
        if (!is_file($file)) {
            throw new \RuntimeException("asset manifest {$file} does not exist; run make assets");
        }
        $urls = json_decode((string) file_get_contents($file), true, flags: JSON_THROW_ON_ERROR);
        if (!is_array($urls) || !is_string($urls['hyper'] ?? null)) {
            throw new \RuntimeException("asset manifest {$file} has no hyper URL");
        }

        return new self(['hyper' => $urls['hyper']]);
    }

    /** @return array{hyper: string} */
    public function urls(): array
    {
        return $this->urls;
    }
}
